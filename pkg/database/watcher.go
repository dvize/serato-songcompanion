package database

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	sc "serato-songcompanion/seratosync"
)

// SessionWatcher monitors Serato's master.sqlite for live track changes.
type SessionWatcher struct {
	// DBPath is the path to master.sqlite. If empty, auto-detected from OS defaults.
	DBPath     string
	UpdateChan chan *sc.SessionEntry
	stopChan   chan struct{}

	db              *sql.DB
	lastTrackID     int64
	lastSessionID   int64
	PlayedFilePaths map[string]bool      // set of file paths played this session
	PlayedAt        map[string]time.Time // file path → last-played time this session
	PlayedCount     int                  // unique tracks played this session
	PlayedGenres    map[string]int       // genre token → play count this session
}

// NewSessionWatcher creates a new watcher. dbPath may be empty for auto-detection.
func NewSessionWatcher(dbPath string) *SessionWatcher {
	if dbPath == "" {
		dbPath = defaultMasterSQLitePath()
	}
	return &SessionWatcher{
		DBPath:          dbPath,
		UpdateChan:      make(chan *sc.SessionEntry, 4),
		stopChan:        make(chan struct{}),
		PlayedFilePaths: make(map[string]bool),
		PlayedAt:        make(map[string]time.Time),
		PlayedGenres:    make(map[string]int),
	}
}

// SessionStats summarizes what has been played in the current session.
type SessionStats struct {
	Count     int      `json:"count"`
	TopGenres []string `json:"top_genres"`
}

// Stats returns current session statistics.
func (w *SessionWatcher) Stats() SessionStats {
	type gv struct {
		name  string
		count int
	}
	var gvs []gv
	for g, c := range w.PlayedGenres {
		gvs = append(gvs, gv{g, c})
	}
	sort.Slice(gvs, func(i, j int) bool {
		if gvs[i].count != gvs[j].count {
			return gvs[i].count > gvs[j].count
		}
		return gvs[i].name < gvs[j].name
	})
	n := 3
	if len(gvs) < n {
		n = len(gvs)
	}
	top := make([]string, n)
	for i := 0; i < n; i++ {
		top[i] = gvs[i].name
	}
	return SessionStats{Count: w.PlayedCount, TopGenres: top}
}

// defaultMasterSQLitePath returns the platform-appropriate path to master.sqlite.
func defaultMasterSQLitePath() string {
	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(localAppData, "Serato", "Library", "master.sqlite")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Serato", "Library", "master.sqlite")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "Serato", "Library", "master.sqlite")
	}
}

// Start begins monitoring in a background goroutine.
func (w *SessionWatcher) Start() {
	log.Printf("SessionWatcher: monitoring %s", w.DBPath)
	go w.loop()
}

// Stop halts monitoring.
func (w *SessionWatcher) Stop() {
	close(w.stopChan)
}

func (w *SessionWatcher) openDB() error {
	if w.db != nil {
		return nil
	}
	// mode=ro — read-only, no immutable flag so SQLite always reads fresh WAL pages.
	// _busy_timeout helps if Serato holds a brief write lock.
	dsn := "file:" + w.DBPath + "?mode=ro&_busy_timeout=500"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	w.db = db
	return nil
}

func (w *SessionWatcher) loop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer func() {
		if w.db != nil {
			w.db.Close()
		}
	}()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *SessionWatcher) check() {
	if err := w.openDB(); err != nil {
		log.Printf("SessionWatcher: open error: %v", err)
		return
	}

	// Find the currently open session (end_time = -1 means session is still active).
	var sessionID int64
	err := w.db.QueryRow(`
		SELECT id FROM history_session
		WHERE end_time = -1
		ORDER BY start_time DESC
		LIMIT 1
	`).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return
	}
	if err != nil {
		log.Printf("SessionWatcher: session query error: %v", err)
		// Connection may be stale — reopen next tick.
		w.db.Close()
		w.db = nil
		return
	}

	// New session started — reset our cursor so we pick up from the beginning.
	if sessionID != w.lastSessionID {
		log.Printf("SessionWatcher: new session id=%d (was %d), resetting cursor", sessionID, w.lastSessionID)
		w.lastSessionID = sessionID
		w.lastTrackID = 0
	}

	// Query ALL entries newer than the last seen id — no played filter.
	// Serato writes the row immediately with played=0; played=1 arrives later
	// (after ~30-60s threshold). Waiting for played=1 causes the exact delay
	// the user observed. We emit on insertion instead, using the latest row
	// per deck (highest id) to handle rapid track switching correctly.
	rows, err := w.db.Query(`
		SELECT id, COALESCE(artist, ''), COALESCE(name, ''), COALESCE(bpm, 0),
		       COALESCE(key, ''), COALESCE(genre, ''), COALESCE(file_name, ''),
		       COALESCE(deck, '')
		FROM history_entry
		WHERE session_id = ?
		  AND id > ?
		ORDER BY id ASC
	`, sessionID, w.lastTrackID)
	if err != nil {
		log.Printf("SessionWatcher: entry query error: %v", err)
		w.db.Close()
		w.db = nil
		return
	}
	defer rows.Close()

	// Collect all new rows; for each deck keep only the latest (last seen wins
	// on rapid switching — intermediate skipped tracks don't get recommendations).
	type row struct {
		id     int64
		artist string
		title  string
		bpm    float64
		key    string
		genre  string
		fpath  string
		deck   string
	}
	latestPerDeck := map[string]*row{}
	var maxID int64

	for rows.Next() {
		r := &row{}
		if err := rows.Scan(&r.id, &r.artist, &r.title, &r.bpm, &r.key, &r.genre, &r.fpath, &r.deck); err != nil {
			log.Printf("SessionWatcher: row scan error: %v", err)
			continue
		}
		if r.id > maxID {
			maxID = r.id
		}
		latestPerDeck[r.deck] = r
	}

	if maxID == 0 {
		return // nothing new
	}

	// Advance cursor past all rows we just read.
	w.lastTrackID = maxID

	// Emit one update per deck (latest track on that deck).
	for _, r := range latestPerDeck {
		entry := &sc.SessionEntry{
			FilePath: r.fpath,
			Title:    r.title,
			Artist:   r.artist,
			BPM:      r.bpm,
			Key:      r.key,
		}
		if r.genre != "" {
			entry.Genres = []string{r.genre}
		}

		log.Printf("SessionWatcher: deck=%q new track id=%d artist=%q title=%q bpm=%.1f key=%q",
			r.deck, r.id, r.artist, r.title, r.bpm, r.key)

		if r.fpath != "" {
			if !w.PlayedFilePaths[r.fpath] {
				w.PlayedCount++
			}
			w.PlayedFilePaths[r.fpath] = true
			w.PlayedAt[r.fpath] = time.Now()
			for _, g := range strings.Split(r.genre, ",") {
				g = strings.ToLower(strings.TrimSpace(g))
				if g != "" {
					w.PlayedGenres[g]++
				}
			}
		}

		select {
		case w.UpdateChan <- entry:
		default:
		}
	}
}
