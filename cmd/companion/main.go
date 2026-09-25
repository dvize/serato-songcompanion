package main

import (
	"encoding/json"
	"log"
	"os"

	"serato-songcompanion/pkg/database"
	"serato-songcompanion/pkg/server"
)

type Config struct {
	DatabasePath  string `json:"database_path"` // empty = platform default (~/Music/_Serato_ etc.)
	HistoryPath   string `json:"history_path"`  // legacy, unused
	SessionsDir   string `json:"sessions_dir"`  // legacy, unused
	MasterSQLite  string `json:"master_sqlite"` // path to Serato master.sqlite; empty = auto-detect
}

func loadConfig() Config {
	// Default config: point at the user's real Serato library, never the
	// process working directory (inside Electron that is Resources/, which
	// has no database V2 file — cwd-based defaults silently load 0 tracks).
	config := Config{
		DatabasePath: database.DefaultDatabasePath(),
	}

	// Try to load from config.json
	f, err := os.Open("config.json")
	if err == nil {
		defer f.Close()
		if err := json.NewDecoder(f).Decode(&config); err != nil {
			log.Printf("Error decoding config.json: %v", err)
		} else {
			log.Println("Loaded configuration from config.json")
		}
	} else {
		log.Println("No config.json found, using defaults")
	}
	// Empty strings in config.json must not clobber defaults
	if config.DatabasePath == "" {
		config.DatabasePath = database.DefaultDatabasePath()
	}
	return config
}

func main() {
	config := loadConfig()

	// 1. Parse Database
	dbPath := config.DatabasePath
	if dbPath == "" {
		dbPath = database.DefaultDatabasePath()
	}
	log.Printf("Parsing Serato database at %s...", dbPath)
	db, err := database.ParseDatabase(dbPath)
	if err != nil {
		log.Printf("Warning: Could not parse database V2: %v", err)
		db = &database.Database{}
	}
	log.Printf("Loaded %d tracks from database", len(db.Tracks))

	// 2. Start Watcher
	log.Printf("Starting session watcher (master.sqlite)...")
	watcher := database.NewSessionWatcher(config.MasterSQLite)
	watcher.Start()
	defer watcher.Stop()

	// 3. Start Server
	srv := server.NewServer(db, watcher)
	srv.Start(8080)
}
