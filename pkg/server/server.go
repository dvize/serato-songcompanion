package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"

	"serato-songcompanion/pkg/database"
	"serato-songcompanion/pkg/recommendation"
	sc "serato-songcompanion/seratosync"
)

// Server represents the HTTP server
type Server struct {
	DB           *database.Database
	Engine       *recommendation.Engine
	Watcher      *database.SessionWatcher
	CurrentTrack *sc.SessionEntry
	trackIndex     map[string]*database.Track // full file path → library track
	trackByName    map[string]*database.Track // bare filename → library track (fallback)
	clients      map[*websocket.Conn]bool
	clientsMu    sync.Mutex
	upgrader     websocket.Upgrader
}

// NewServer creates a new server
func NewServer(db *database.Database, watcher *database.SessionWatcher) *Server {
	engine := recommendation.NewEngine(db)

	// Build O(1) lookup indexes
	byPath := make(map[string]*database.Track, len(db.Tracks))
	byName := make(map[string]*database.Track, len(db.Tracks))
	for i := range db.Tracks {
		t := &db.Tracks[i]
		byPath[t.FilePath] = t
		byName[filepath.Base(t.FilePath)] = t
	}

	return &Server{
		DB:          db,
		Engine:      engine,
		Watcher:     watcher,
		trackIndex:  byPath,
		trackByName: byName,
		clients:     make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Start starts the server
func (s *Server) Start(port int) {
	go s.listenToWatcher()

	http.HandleFunc("/api/current-track", s.handleCurrentTrack)
	http.HandleFunc("/api/recommendations", s.handleRecommendations)
	http.HandleFunc("/api/reveal", s.handleReveal)
	http.HandleFunc("/ws", s.handleWebSocket)

	// Serve static files (frontend)
	fs := http.FileServer(http.Dir("./web"))
	http.Handle("/", fs)

	addr := fmt.Sprintf(":%d", port)
	url := fmt.Sprintf("http://localhost:%d", port)
	log.Printf("Server starting on %s", url)
	// Skip auto-opening a browser when running under Electron (it manages the window).
	if os.Getenv("SERATO_NO_BROWSER") != "1" {
		go openBrowser(url)
	}
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Could not open browser: %v", err)
	}
}

func (s *Server) listenToWatcher() {
	for entry := range s.Watcher.UpdateChan {
		// Enrich Energy and Genre from the library database.
		// history_entry only stores the bare filename, not the full path,
		// so try full-path first then fall back to filename match.
		if t, ok := s.trackIndex[entry.FilePath]; ok {
			s.enrichFromTrack(entry, t)
		} else if t, ok := s.trackByName[filepath.Base(entry.FilePath)]; ok {
			s.enrichFromTrack(entry, t)
			entry.FilePath = t.FilePath // upgrade to full path for drag-and-drop
		}
		log.Printf("New track detected: %s - %s (energy=%d)", entry.Artist, entry.Title, entry.Energy)
		s.CurrentTrack = entry
		s.broadcast("current_track", entry)
	}
}

func (s *Server) enrichFromTrack(entry *sc.SessionEntry, t *database.Track) {
	if entry.Energy == 0 {
		entry.Energy = t.Energy
	}
	if len(entry.Genres) == 0 && t.Genre != "" {
		entry.Genres = []string{t.Genre}
	}
}

func (s *Server) handleCurrentTrack(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.CurrentTrack)
}

func (s *Server) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Core track params
	bpmStr := q.Get("bpm")
	key := q.Get("key")
	genre := q.Get("genre")
	energyStr := q.Get("energy")

	var bpm float64
	if bpmStr != "" {
		bpm, _ = strconv.ParseFloat(bpmStr, 64)
	}
	var energy int
	if energyStr != "" {
		energy, _ = strconv.Atoi(energyStr)
	}

	// If no params provided and we have a current track, use it
	if bpm == 0 && key == "" && genre == "" && s.CurrentTrack != nil {
		bpm = s.CurrentTrack.BPM
		key = s.CurrentTrack.Key
		if len(s.CurrentTrack.Genres) > 0 {
			genre = s.CurrentTrack.Genres[0]
		}
		energy = s.CurrentTrack.Energy
	}

	// Filter toggles
	matchKey := parseBool(q.Get("match_key"), true)
	matchGenre := parseBool(q.Get("match_genre"), false)
	energyStep := parseBool(q.Get("energy_step"), false)
	hidePlayed := parseBool(q.Get("hide_played"), false)

	// BPM range: prefer absolute min/max, fallback to percentage
	bpmMin, _ := strconv.ParseFloat(q.Get("bpm_min"), 64)
	bpmMax, _ := strconv.ParseFloat(q.Get("bpm_max"), 64)
	bpmPct := 0.08 // default 8%
	if s := q.Get("bpm_pct"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			bpmPct = v / 100.0 // frontend sends as integer percent e.g. "8"
		}
	}

	// Max results
	maxResults := 50 // sensible default
	if s := q.Get("max_results"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			maxResults = v
		}
	}

	// Year range (0 = no limit / "All")
	yearMin, _ := strconv.Atoi(q.Get("year_min"))
	yearMax, _ := strconv.Atoi(q.Get("year_max"))
	yearSource := q.Get("year_source") // "added" | "year"; defaults to "added" in engine

	// Build exclusion set
	var excludePaths map[string]bool
	if hidePlayed && s.Watcher != nil {
		excludePaths = s.Watcher.PlayedFilePaths
	}

	criteria := recommendation.Criteria{
		SourceBPM:        bpm,
		SourceKey:        key,
		SourceGenre:      genre,
		SourceEnergy:     energy,
		BPMMin:           bpmMin,
		BPMMax:           bpmMax,
		BPMRangePercent:  bpmPct,
		MatchKey:         matchKey,
		MatchGenre:       matchGenre,
		EnergyStep:       energyStep,
		YearMin:          yearMin,
		YearMax:          yearMax,
		YearSource:       yearSource,
		ExcludeFilePaths: excludePaths,
		MaxResults:       maxResults,
	}

	recs := s.Engine.GetRecommendations(criteria)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recs)
}

// parseBool parses a query param as bool, returning defaultVal if the param is empty.
func parseBool(s string, defaultVal bool) bool {
	if s == "" {
		return defaultVal
	}
	return s == "1" || s == "true"
}

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Use PowerShell to invoke Explorer's select-file dialog.
		// This handles spaces and special characters in paths correctly.
		ps := `explorer.exe /select,"` + req.Path + `"`
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	case "darwin":
		cmd = exec.Command("open", "-R", req.Path)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(req.Path))
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error revealing file: %v", err)
		http.Error(w, "Failed to reveal file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	s.clientsMu.Lock()
	s.clients[conn] = true
	s.clientsMu.Unlock()

	// Send current state immediately
	if s.CurrentTrack != nil {
		msg := map[string]interface{}{
			"type": "current_track",
			"data": s.CurrentTrack,
		}
		conn.WriteJSON(msg)
	}

	// Keep connection alive / handle close
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			s.clientsMu.Lock()
			delete(s.clients, conn)
			s.clientsMu.Unlock()
			conn.Close()
			break
		}
	}
}

func (s *Server) broadcast(msgType string, data interface{}) {
	msg := map[string]interface{}{
		"type": msgType,
		"data": data,
	}

	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	for client := range s.clients {
		if err := client.WriteJSON(msg); err != nil {
			log.Printf("WebSocket write error: %v", err)
			client.Close()
			delete(s.clients, client)
		}
	}
}
