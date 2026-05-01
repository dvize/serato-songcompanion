// checkdates: diagnostic — prints DateAdded and Year for the first 20 tracks
// in the Serato library so we can confirm tadd parsing is correct.
// Run: go run ./cmd/checkdates/
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	sc "serato-songcompanion/seratosync"
)

func main() {
	dbPath := dbDefault()
	fmt.Println("Database:", dbPath)

	chunks, err := sc.ReadFileChunks(dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
		os.Exit(1)
	}

	// Collect all unique DateAdded years to show range of data
	yearCounts := map[int]int{}
	total := 0
	for _, ch := range chunks {
		if ch.Tag != "otrk" {
			continue
		}
		total++
		for _, c := range ch.Children {
			if c.Tag != "tadd" {
				continue
			}
			str := strings.TrimSpace(strings.TrimRight(sc.DecodeUTF16BE(c.Data), "\x00"))
			if ts, err := strconv.ParseInt(str, 10, 64); err == nil && ts > 946684800 {
				yearCounts[time.Unix(ts, 0).Year()]++
			}
		}
	}
	fmt.Printf("\nTotal otrk chunks: %d\nDateAdded year distribution:\n", total)
	for y := 2015; y <= 2030; y++ {
		if n, ok := yearCounts[y]; ok {
			fmt.Printf("  %d: %d tracks\n", y, n)
		}
	}

	printed := 0
	for _, ch := range chunks {
		if printed >= 20 {
			break
		}
		if ch.Tag != "otrk" {
			continue
		}
		var title string
		var taddRaw []byte
		var taddTS uint32

		for _, c := range ch.Children {
			if c.Tag == "tadd" {
				taddRaw = c.Data
				if len(c.Data) >= 4 {
					taddTS = binary.BigEndian.Uint32(c.Data[len(c.Data)-4:])
				}
				continue
			}
			val := sc.DecodeUTF16BE(c.Data)
			switch c.Tag {
			case "tsng":
				title = val
			}
		}

		if title == "" {
			continue
		}

		fmt.Printf("%-40s | utf16be=%q → ", trunc(title, 40), sc.DecodeUTF16BE(taddRaw))

		str := strings.TrimSpace(strings.TrimRight(sc.DecodeUTF16BE(taddRaw), "\x00"))
		if ts, err := strconv.ParseInt(str, 10, 64); err == nil && ts > 0 {
			t := time.Unix(ts, 0)
			fmt.Printf("DateAdded=%s (year %d)\n", t.Format("2006-01-02 15:04:05"), t.Year())
		} else {
			fmt.Printf("parse failed (raw uint32=%d)\n", taddTS)
		}
		printed++
	}
	fmt.Printf("\nChecked %d tracks.\n", printed)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func dbDefault() string {
	switch runtime.GOOS {
	case "windows":
		home := os.Getenv("USERPROFILE")
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		for _, drive := range []string{"E", "D", "F", "G"} {
			p := filepath.Join(drive+":\\", "_Serato_", "database V2")
			if info, err := os.Stat(p); err == nil && info.Size() > 1024 {
				return p
			}
		}
		return filepath.Join(home, "Music", "_Serato_", "database V2")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Music", "_Serato_", "database V2")
	}
}
