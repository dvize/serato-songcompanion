package recommendation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"serato-songcompanion/pkg/database"
)

// Engine handles song recommendations
type Engine struct {
	DB *database.Database
}

// NewEngine creates a new recommendation engine
func NewEngine(db *database.Database) *Engine {
	return &Engine{DB: db}
}

// Criteria defines the filters for recommendations
type Criteria struct {
	SourceBPM    float64
	SourceKey    string
	SourceGenre  string
	SourceEnergy int

	// BPM filtering: use BPMMin/BPMMax for absolute range (0 = no limit)
	// or BPMRangePercent for a percentage tolerance around SourceBPM.
	BPMMin          float64 // 0 = no min
	BPMMax          float64 // 0 = no max
	BPMRangePercent float64 // e.g. 0.08 for ±8% (used if BPMMin/Max are both 0)

	MatchKey   bool
	MatchGenre bool

	// EnergyMode controls how candidate energy is compared to SourceEnergy:
	//   "any"    — no energy filter
	//   "exact"  — only tracks at SourceEnergy
	//   "within" — only tracks within ±EnergyRange of SourceEnergy
	//   "higher" — only tracks at or above SourceEnergy
	//   "lower"  — only tracks at or below SourceEnergy
	// Legacy: EnergyStep=true maps to "within" with range 1.
	EnergyMode  string
	EnergyRange int
	EnergyStep  bool // deprecated, kept for API compat

	// GenreCount: when > 0, only the first N genres of the source genre string
	// are considered (comma-separated lists on both sides).
	GenreCount int
	// GenreMode: "any" (default) = at least one genre must match (scored);
	// "all" = every considered source genre must match (hard filter).
	GenreMode string

	// BPMHalfDouble: allow half/double-time BPM matches (default via server settings).
	BPMHalfDouble bool

	// KeyTiers: enable/disable Camelot compatibility tiers
	// (same/relative/adjacent/energy_boost/dominant). nil = all enabled.
	KeyTiers map[string]bool
	// KeyWeights: score contribution per tier. nil = built-in defaults.
	KeyWeights map[string]float64

	// CooldownPaths: file path → last-played time. Cooldown > 0 hides tracks
	// played within that duration (stronger than ExcludeFilePaths).
	CooldownPaths map[string]time.Time
	Cooldown      time.Duration

	// YearMin / YearMax: filter by year (0 = no limit / "All years").
	// YearSource controls which year to compare:
	//   "added"  — year the track was added to Serato (DateAdded)
	//   "year"   — release year from the track's ID3 TYER tag
	// Defaults to "added" when empty.
	YearMin    int
	YearMax    int
	YearSource string // "added" | "year"

	// ExcludeFilePaths: set of file paths already played this session to hide
	ExcludeFilePaths map[string]bool

	// MaxResults: if > 0, cap results to this count (0 = unlimited)
	MaxResults int
}

// Recommendation represents a recommended track with a score
type Recommendation struct {
	Track  database.Track
	Score  float64
	Reason string
}

// GetRecommendations returns a list of recommended tracks sorted by score descending
func (e *Engine) GetRecommendations(c Criteria) []Recommendation {
	var recs []Recommendation

	for _, t := range e.DB.Tracks {
		// --- Exclusion filters (hard filters, not scoring) ---

		// Skip tracks already played this session
		if c.ExcludeFilePaths != nil && c.ExcludeFilePaths[t.FilePath] {
			continue
		}

		// Cooldown: hide tracks played within the cooldown window
		if c.Cooldown > 0 && c.CooldownPaths != nil {
			if ts, ok := c.CooldownPaths[t.FilePath]; ok && time.Since(ts) < c.Cooldown {
				continue
			}
		}

		// Energy filter
		if c.SourceEnergy > 0 && t.Energy > 0 {
			mode := c.EnergyMode
			if mode == "" {
				if c.EnergyStep {
					mode = "within"
				} else {
					mode = "any"
				}
			}
			diff := t.Energy - c.SourceEnergy
			if diff < 0 {
				diff = -diff
			}
			switch mode {
			case "exact":
				if t.Energy != c.SourceEnergy {
					continue
				}
			case "within":
				r := c.EnergyRange
				if r <= 0 {
					r = 1
				}
				if diff > r {
					continue
				}
			case "higher":
				if t.Energy < c.SourceEnergy {
					continue
				}
			case "lower":
				if t.Energy > c.SourceEnergy {
					continue
				}
			}
		}

		// Year filter (0 = no limit; unknown years are always included)
		if c.YearMin > 0 || c.YearMax > 0 {
			src := c.YearSource
			if src == "" {
				src = "added"
			}
			var year int
			switch src {
			case "year":
				year = t.Year // 0 if tag not set
			default: // "added"
				if !t.DateAdded.IsZero() {
					year = t.DateAdded.Year()
				}
			}
			// Only filter when we actually have a year value
			if year > 0 {
				if c.YearMin > 0 && year < c.YearMin {
					continue
				}
				if c.YearMax > 0 && year > c.YearMax {
					continue
				}
			}
		}

		score := 0.0
		reasons := []string{}

		// --- BPM Check ---
		if c.SourceBPM > 0 && t.BPM > 0 {
			inRange := false

			if c.BPMMin > 0 || c.BPMMax > 0 {
				// Absolute BPM range
				aboveMin := c.BPMMin == 0 || t.BPM >= c.BPMMin
				belowMax := c.BPMMax == 0 || t.BPM <= c.BPMMax
				inRange = aboveMin && belowMax
				if inRange {
					diff := math.Abs(t.BPM - c.SourceBPM)
					if diff <= c.SourceBPM*0.01 { // within 1% = very close match
						score += 10
						reasons = append(reasons, "BPM Match")
					} else {
						score += 8
						reasons = append(reasons, "BPM Range")
					}
				}
			} else if c.BPMRangePercent > 0 {
				// Percentage tolerance
				limit := c.SourceBPM * c.BPMRangePercent
				diff := math.Abs(t.BPM - c.SourceBPM)
				if diff <= limit {
					score += 10
					reasons = append(reasons, "BPM Match")
					inRange = true
				} else if c.BPMHalfDouble {
					// Half/double BPM
					halfBPM := c.SourceBPM / 2
					doubleBPM := c.SourceBPM * 2
					if math.Abs(t.BPM-halfBPM) <= halfBPM*c.BPMRangePercent {
						score += 8
						reasons = append(reasons, "Half BPM")
						inRange = true
					} else if math.Abs(t.BPM-doubleBPM) <= doubleBPM*c.BPMRangePercent {
						score += 8
						reasons = append(reasons, "Double BPM")
						inRange = true
					}
				}
			}
			_ = inRange
		}

		// --- Key Check ---
		if c.MatchKey && c.SourceKey != "" && t.Key != "" {
			compat, relation := camelotCompatibility(c.SourceKey, t.Key)
			if compat && tierEnabled(c.KeyTiers, relation) {
				w := tierWeight(c.KeyWeights, relation)
				score += w
				reasons = append(reasons, tierLabel(relation))
			}
		}

		// --- Genre Check ---
		if c.MatchGenre && c.SourceGenre != "" && t.Genre != "" {
			src := splitGenres(c.SourceGenre)
			if c.GenreCount > 0 && c.GenreCount < len(src) {
				src = src[:c.GenreCount]
			}
			tg := splitGenres(t.Genre)
			matches := 0
			for _, s := range src {
				for _, g := range tg {
					if strings.Contains(g, s) || strings.Contains(s, g) {
						matches++
						break
					}
				}
			}
			if matches > 0 && (c.GenreMode != "all" || matches >= len(src)) {
				score += 5 * float64(matches)
				label := "Genre Match"
				if matches > 1 {
					label = fmt.Sprintf("Genre Match ×%d", matches)
				}
				reasons = append(reasons, label)
			}
		}

		if score > 0 {
			recs = append(recs, Recommendation{
				Track:  t,
				Score:  score,
				Reason: strings.Join(reasons, ", "),
			})
		}
	}

	// Shuffle before sorting so that tracks with equal scores appear in random
	// order rather than always favouring whichever tracks were parsed first.
	rand.Shuffle(len(recs), func(i, j int) { recs[i], recs[j] = recs[j], recs[i] })

	// Sort by score descending
	sort.Slice(recs, func(i, j int) bool {
		return recs[i].Score > recs[j].Score
	})

	// Cap results
	if c.MaxResults > 0 && len(recs) > c.MaxResults {
		recs = recs[:c.MaxResults]
	}

	return recs
}

// camelotCompatibility checks Camelot wheel key compatibility.
// Returns (compatible bool, relation string).
//
// Compatible relations (all confirmed by Mixed In Key harmonic mixing guide):
//
//	"same"         — identical key (e.g. 8A → 8A)                              score: +10
//	"relative"     — same number, different mode (e.g. 8A → 8B)               score: +9
//	"adjacent"     — ±1 on the wheel, same mode (e.g. 8A → 7A or 9A)         score: +7
//	"energy_boost" — +7 positions same mode (a.k.a. Armin Van Buuren -5 var)  score: +6
//	                 e.g. 2A → 9A  (2+7=9)
//	"dominant"     — +3 positions, mode flipped (e.g. 2A → 5B)               score: +5
func camelotCompatibility(k1, k2 string) (bool, string) {
	c1, ok1 := parseCamelot(k1)
	c2, ok2 := parseCamelot(k2)

	if !ok1 || !ok2 {
		// Fall back to exact string match
		if strings.EqualFold(k1, k2) {
			return true, "same"
		}
		return false, ""
	}

	// Same key
	if c1.num == c2.num && c1.minor == c2.minor {
		return true, "same"
	}

	// Relative mode: same number, different A/B
	if c1.num == c2.num {
		return true, "relative"
	}

	// Adjacent on the wheel (±1, wrapping 1↔12), same mode
	if c1.minor == c2.minor {
		diff := wheelDiff(c1.num, c2.num)
		if diff == 1 {
			return true, "adjacent"
		}
		// Energy boost: +7 (same as -5) on wheel, same mode
		// e.g. 2A→9A: 9-2=7; or 2A→7A (going backwards): min(7,5)=5... no, we want exactly +7
		// We check both directions: forward +7 and backward +7 (= forward +5... no)
		// Forward: (c1+7-1)%12+1; check if that equals c2
		forward7 := (c1.num-1+7)%12 + 1
		backward7 := (c1.num-1+12-7)%12 + 1 // -7 = +5 going back
		if c2.num == forward7 || c2.num == backward7 {
			return true, "energy_boost"
		}
	}

	// Dominant: +3 on wheel with mode flip
	// e.g. 2A → 5B: (2+3)=5, flip A→B
	if c1.minor != c2.minor {
		forward3 := (c1.num-1+3)%12 + 1
		backward3 := (c1.num-1+12-3)%12 + 1
		if c2.num == forward3 || c2.num == backward3 {
			return true, "dominant"
		}
	}

	return false, ""
}

// wheelDiff returns the minimum circular distance between two Camelot wheel numbers (1-12).
func wheelDiff(a, b int) int {
	d := a - b
	if d < 0 {
		d = -d
	}
	if d > 6 {
		d = 12 - d
	}
	return d
}

// tierEnabled reports whether a Camelot compatibility tier is enabled.
// Unknown/nil maps default to enabled.
func tierEnabled(tiers map[string]bool, relation string) bool {
	if tiers == nil {
		return true
	}
	v, ok := tiers[relation]
	return !ok || v
}

var defaultTierWeights = map[string]float64{
	"same":         10,
	"relative":     9,
	"adjacent":     7,
	"energy_boost": 6,
	"dominant":     5,
}

// tierWeight returns the score weight for a tier, defaulting to built-ins.
func tierWeight(weights map[string]float64, relation string) float64 {
	if w, ok := weights[relation]; ok {
		return w
	}
	return defaultTierWeights[relation]
}

var tierLabels = map[string]string{
	"same":         "Same Key",
	"relative":     "Relative (A↔B)",
	"adjacent":     "Adjacent Key",
	"energy_boost": "Energy Boost (+7)",
	"dominant":     "Dominant (+3)",
}

func tierLabel(relation string) string {
	if l, ok := tierLabels[relation]; ok {
		return l
	}
	return relation
}

// splitGenres splits a comma-separated genre string into lowercased tokens.
func splitGenres(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

type camelotKey struct {
	num   int  // 1–12
	minor bool // true=A (minor), false=B (major)
}

// parseCamelot parses Camelot notation ("8A", "12B") or
// Open Key notation ("5m", "3d") into a camelotKey.
// Returns (key, true) on success, (zero, false) on failure.
func parseCamelot(k string) (camelotKey, bool) {
	k = strings.TrimSpace(k)
	if len(k) < 2 {
		return camelotKey{}, false
	}

	suffix := strings.ToLower(k[len(k)-1:])
	numStr := k[:len(k)-1]

	// Handle Camelot: suffix is 'a' or 'b'
	if suffix == "a" || suffix == "b" {
		n, err := strconv.Atoi(numStr)
		if err != nil || n < 1 || n > 12 {
			return camelotKey{}, false
		}
		return camelotKey{num: n, minor: suffix == "a"}, true
	}

	// Handle Open Key: suffix is 'm' (minor) or 'd' (major)
	// Open Key 1m = Camelot 6A: camelot = ((openKey - 1 + 5) % 12) + 1
	if suffix == "m" || suffix == "d" {
		n, err := strconv.Atoi(numStr)
		if err != nil || n < 1 || n > 12 {
			return camelotKey{}, false
		}
		camelotNum := ((n-1+5)%12 + 1)
		return camelotKey{num: camelotNum, minor: suffix == "m"}, true
	}

	return camelotKey{}, false
}
