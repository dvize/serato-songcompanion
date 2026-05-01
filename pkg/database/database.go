package database

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	sc "serato-songcompanion/seratosync"
)

// Track represents a song in the Serato database
type Track struct {
	ID        int
	FilePath  string
	Title     string
	Artist    string
	Album     string
	Genre     string
	Key       string
	BPM       float64
	Energy    int       // Mixed In Key energy level 1-10, parsed from tgrp "Energy N"
	DateAdded time.Time // parsed from tadd (Unix timestamp uint32 big-endian)
	Year      int       // release year from tyer ID3 tag (e.g. 2023)
}

// Database represents the parsed Serato database
type Database struct {
	Tracks []Track
}

// DefaultDatabasePath returns the platform-appropriate path to Serato's 'database V2' file.
// Serato stores the main library database in the _Serato_ folder next to the user's music.
func DefaultDatabasePath() string {
	switch runtime.GOOS {
	case "windows":
		// Try %USERPROFILE%\Music\_Serato_\database V2 first (default Music library location)
		home := os.Getenv("USERPROFILE")
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		candidates := []string{
			filepath.Join(home, "Music", "_Serato_", "database V2"),
		}
		// Also try common external drive letters and OneDrive Music paths
		for _, drive := range []string{"D", "E", "F", "G"} {
			candidates = append(candidates, filepath.Join(drive+":\\", "_Serato_", "database V2"))
		}
		candidates = append(candidates,
			filepath.Join(home, "OneDrive", "Music", "_Serato_", "database V2"),
		)
		for _, c := range candidates {
			info, err := os.Stat(c)
			if err == nil && info.Size() > 1024 { // must be >1KB to be a real library
				return c
			}
		}
		// Fall back to Music location even if it doesn't exist yet
		return filepath.Join(home, "Music", "_Serato_", "database V2")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Music", "_Serato_", "database V2")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Music", "_Serato_", "database V2")
	}
}

// ParseDatabase parses the 'database V2' file.
// database V2 uses named 4-byte ASCII child chunk tags with UTF-16 Big-Endian values.
// pfil paths in the database are relative to the drive root (e.g. "Music/foo.mp3").
func ParseDatabase(path string) (*Database, error) {
	chunks, err := sc.ReadFileChunks(path)
	if err != nil {
		return nil, err
	}

	// pfil paths are relative to the drive root where the database lives.
	// Windows: database at E:\_Serato_\database V2  →  root = E:\
	// macOS:   database at /Users/x/Music/_Serato_/database V2  →  root = /
	//          (pfil stores paths like "Users/x/Music/Artist/track.mp3")
	var dbRoot string
	if runtime.GOOS == "windows" {
		dbRoot = filepath.VolumeName(path) + string(filepath.Separator)
	} else {
		dbRoot = string(filepath.Separator)
	}

	db := &Database{}
	for _, ch := range chunks {
		if ch.Tag == "otrk" {
			t := parseTrack(ch, dbRoot)
			if t.FilePath != "" {
				db.Tracks = append(db.Tracks, t)
			}
		}
	}

	return db, nil
}

// parseTrack parses a single otrk chunk into a Track.
// dbRoot is the root directory used to resolve relative pfil paths
// (Serato stores paths relative to the drive root, e.g. "Music/..." means "<drive>\Music\...").
func parseTrack(chunk sc.Chunk, dbRoot string) Track {
	var t Track

	// otrk children are named sub-chunks with 4-byte ASCII tags.
	// Values are UTF-16 Big-Endian encoded strings.
	// Known tags:
	//   pfil = file path (relative to drive root, e.g. "Music/foo.mp3")
	//   tsng = song title  (NOT ttit — Serato uses tsng)
	//   tart = artist
	//   talb = album
	//   tgen = genre (comma-separated)
	//   tkey = key (Camelot notation, e.g. "8A")
	//   tbpm = BPM as a decimal string (e.g. "128.00")
	//   tyer = release year (e.g. "2023")
	//   tgrp = grouping — Mixed In Key stores "Energy N" here
	//   tlen = length formatted string (e.g. "03:25.71") — not used numerically

	for _, child := range chunk.Children {
		// tadd is a UTF-16BE encoded decimal string of a Unix timestamp,
		// e.g. "1581523567"  (NOT a raw binary uint32 as some docs claim)
		if child.Tag == "tadd" {
			str := strings.TrimSpace(strings.TrimRight(sc.DecodeUTF16BE(child.Data), "\x00"))
			if ts, err := strconv.ParseInt(str, 10, 64); err == nil && ts > 0 {
				t.DateAdded = time.Unix(ts, 0)
			}
			continue
		}

		val := strings.TrimRight(sc.DecodeUTF16BE(child.Data), "\x00")
		switch child.Tag {
		case "pfil":
			if val != "" {
				// pfil is relative to the drive root (e.g. "Music/foo.mp3")
				// Convert forward slashes to OS separator and join with dbRoot
				rel := filepath.FromSlash(val)
				t.FilePath = filepath.Join(dbRoot, rel)
			}
		case "tsng":
			// Serato stores the track title in tsng (song name), not ttit
			t.Title = val
		case "tart":
			t.Artist = val
		case "talb":
			t.Album = val
		case "tgen":
			t.Genre = val
		case "tyer":
			// Release year — may be a 4-digit year or a full date like "2023-04-01"
			yearStr := strings.TrimSpace(val)
			if len(yearStr) >= 4 {
				yearStr = yearStr[:4]
			}
			if y, err := strconv.Atoi(yearStr); err == nil && y > 1900 && y < 2100 {
				t.Year = y
			}
		case "tkey":
			t.Key = val
		case "tgrp":
			// Mixed In Key stores energy as "Energy N" (N = 1-10)
			if strings.HasPrefix(val, "Energy ") {
				n := strings.TrimPrefix(val, "Energy ")
				if v, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
					t.Energy = v
				}
			}
		case "tbpm":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				t.BPM = f
			}
		}
	}

	return t
}
