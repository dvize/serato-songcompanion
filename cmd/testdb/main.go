package main

import (
	"fmt"
	"os"
	"path/filepath"
	sc "serato-songcompanion/seratosync"
)

func main() {
	root, _ := os.Getwd()
	dbPath := filepath.Join(root, "database V2")

	if _, err := os.Stat(dbPath); err == nil {
		chunks, err := sc.ReadFileChunks(dbPath)
		if err != nil {
			fmt.Printf("Error reading chunks: %v\n", err)
			return
		}
		fmt.Printf("First chunk tags: ")
		for i := 0; i < len(chunks) && i < 12; i++ {
			fmt.Printf("%s ", chunks[i].Tag)
		}
		fmt.Println()
		
		// Check for 'otrk' (track) tags which are common in Serato DB
		trackCount := 0
		for _, ch := range chunks {
			if ch.Tag == "otrk" {
				trackCount++
			}
		}
		fmt.Printf("Found %d tracks (otrk)\n", trackCount)

		if trackCount > 0 {
			// Try to parse one track
			for _, ch := range chunks {
				if ch.Tag == "otrk" {
					// It usually contains an 'adat' chunk
					// We can reuse ParseSessionEntryPair logic if it's similar
					// But let's just see what's inside
					fmt.Printf("First track children: ")
					for _, child := range ch.Children {
						fmt.Printf("%s ", child.Tag)
					}
					fmt.Println()
					break
				}
			}
		}
	} else {
		fmt.Printf("database V2 not found at %s\n", dbPath)
	}
}
