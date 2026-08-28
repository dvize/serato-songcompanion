package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Settings holds user preferences persisted to a writable per-OS config file.
// The legacy root config.json (database paths) is untouched; this file only
// holds matching defaults, window sizing, and UI preferences.
type Settings struct {
	// Matching defaults
	EnergyMode    string             `json:"energy_mode"`     // "any" | "exact" | "within" | "higher" | "lower"
	EnergyRange   int                `json:"energy_range"`    // ±N when EnergyMode == "within"
	GenreCount    int                `json:"genre_count"`     // match against first N source genres (0 = all)
	GenreMode     string             `json:"genre_mode"`      // "any" | "all"
	BPMPercent    float64            `json:"bpm_percent"`     // BPM tolerance percent (e.g. 8 = ±8%)
	BPMHalfDouble bool               `json:"bpm_half_double"` // allow half/double BPM matches
	KeyTiers      map[string]bool    `json:"key_tiers"`       // same/relative/adjacent/energy_boost/dominant
	KeyWeights    map[string]float64 `json:"key_weights"`     // score per tier
	MaxResults    int                `json:"max_results"`
	HidePlayed    bool               `json:"hide_played"`
	CooldownMin   int                `json:"cooldown_min"` // hide tracks played within N minutes (0 = off)

	// Window
	CollapsedWidth  int    `json:"collapsed_width"`
	CollapsedHeight int    `json:"collapsed_height"`
	ExpandedWidth   int    `json:"expanded_width"`
	ExpandedHeight  int    `json:"expanded_height"`
	DimEnabled      bool   `json:"dim_enabled"`
	DimOpacity      int    `json:"dim_opacity"` // percent opacity while dimmed
	DimDelaySec     int    `json:"dim_delay_sec"`
	Hotkey          string `json:"hotkey"`    // Electron accelerator, "" = off
	TrayMode        bool   `json:"tray_mode"` // macOS tray icon toggle

	// General
	SessionStats bool `json:"session_stats"` // show "tonight" strip in header
}

// Default returns the default settings.
func Default() *Settings {
	return &Settings{
		EnergyMode:      "within",
		EnergyRange:     1,
		GenreCount:      0,
		GenreMode:       "any",
		BPMPercent:      8,
		BPMHalfDouble:   true,
		KeyTiers:        map[string]bool{"same": true, "relative": true, "adjacent": true, "energy_boost": true, "dominant": true},
		KeyWeights:      map[string]float64{"same": 10, "relative": 9, "adjacent": 7, "energy_boost": 6, "dominant": 5},
		MaxResults:      50,
		HidePlayed:      false,
		CooldownMin:     0,
		CollapsedWidth:  340,
		CollapsedHeight: 64,
		ExpandedWidth:   860,
		ExpandedHeight:  720,
		DimEnabled:      true,
		DimOpacity:      50,
		DimDelaySec:     8,
		Hotkey:          "",
		TrayMode:        false,
		SessionStats:    true,
	}
}

// Path returns the platform-appropriate settings file path.
// Overridable with SERATO_SETTINGS_PATH (useful for dev/portable installs).
func Path() string {
	if p := os.Getenv("SERATO_SETTINGS_PATH"); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(appData, "Serato Companion", "settings.json")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Serato Companion", "settings.json")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "Serato Companion", "settings.json")
	}
}

// Load reads settings from disk, falling back to defaults for missing keys.
// Returns (settings, loadedFromFile).
func Load() (*Settings, bool) {
	s := Default()
	data, err := os.ReadFile(Path())
	if err != nil {
		return s, false
	}
	if err := json.Unmarshal(data, s); err != nil {
		fmt.Printf("settings: corrupt settings file, using defaults: %v\n", err)
		return Default(), false
	}
	s.clamp()
	return s, true
}

// Save persists settings atomically (write temp file, rename over target).
func (s *Settings) Save() error {
	s.clamp()
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// clamp restricts values to sane ranges.
func (s *Settings) clamp() {
	if s.BPMPercent < 1 {
		s.BPMPercent = 8
	}
	if s.BPMPercent > 50 {
		s.BPMPercent = 50
	}
	if s.EnergyRange < 1 {
		s.EnergyRange = 1
	}
	if s.EnergyRange > 9 {
		s.EnergyRange = 9
	}
	if s.GenreCount < 0 {
		s.GenreCount = 0
	}
	if s.GenreCount > 20 {
		s.GenreCount = 20
	}
	if s.MaxResults < 0 {
		s.MaxResults = 50
	}
	if s.MaxResults > 500 {
		s.MaxResults = 500
	}
	if s.CooldownMin < 0 {
		s.CooldownMin = 0
	}
	if s.CooldownMin > 240 {
		s.CooldownMin = 240
	}
	if s.CollapsedWidth < 200 {
		s.CollapsedWidth = 340
	}
	if s.CollapsedHeight < 32 {
		s.CollapsedHeight = 64
	}
	if s.ExpandedWidth < 400 {
		s.ExpandedWidth = 860
	}
	if s.ExpandedHeight < 300 {
		s.ExpandedHeight = 720
	}
	if s.DimOpacity < 5 {
		s.DimOpacity = 5
	}
	if s.DimOpacity > 100 {
		s.DimOpacity = 100
	}
	if s.DimDelaySec < 1 {
		s.DimDelaySec = 1
	}
	if s.DimDelaySec > 120 {
		s.DimDelaySec = 120
	}
	switch s.EnergyMode {
	case "any", "exact", "within", "higher", "lower":
	default:
		s.EnergyMode = "within"
	}
	switch s.GenreMode {
	case "any", "all":
	default:
		s.GenreMode = "any"
	}
	if s.KeyTiers == nil {
		s.KeyTiers = Default().KeyTiers
	}
	if s.KeyWeights == nil {
		s.KeyWeights = Default().KeyWeights
	}
}
