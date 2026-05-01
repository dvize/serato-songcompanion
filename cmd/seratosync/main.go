package main

import (
    "encoding/json"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "sort"

    sc "serato-songcompanion/seratosync"
)

func main() {
    root, _ := os.Getwd()
    hdbPath := filepath.Join(root, "history.database")
    sessionsDir := filepath.Join(root, "Sessions")

    if _, err := os.Stat(hdbPath); err == nil {
        chunks, _ := sc.ReadFileChunks(hdbPath)
        fmt.Printf("First chunk tags: ")
        for i := 0; i < len(chunks) && i < 12; i++ {
            fmt.Printf("%s ", chunks[i].Tag)
        }
        fmt.Println()
        // Debug: check for embedded 'adat' inside first oses
        for _, ch := range chunks {
            if ch.Tag == "oses" {
                adats := sc.ExtractADATPayloads(ch.Data)
                fmt.Printf("First oses has %d embedded adat\n", len(adats))
                break
            }
        }
        hdb, err := sc.ParseHistoryDatabase(hdbPath)
        if err != nil {
            log.Fatalf("parse history.database: %v", err)
        }
        fmt.Printf("History version: %s\n", hdb.Version)
        fmt.Printf("Sessions indexed: %d\n", len(hdb.Sessions))
        lim := 5
        if len(hdb.Sessions) < lim {
            lim = len(hdb.Sessions)
        }
        for i := 0; i < lim; i++ {
            s := hdb.Sessions[i]
            fmt.Printf("- %s | device=%s | tracks~%d\n", s.DateText, s.Device, s.TrackCount)
        }
    }

    entries := []string{}
    if f, err := os.ReadDir(sessionsDir); err == nil {
        for _, e := range f {
            if !e.IsDir() && filepath.Ext(e.Name()) == ".session" {
                entries = append(entries, filepath.Join(sessionsDir, e.Name()))
            }
        }
    }
    sort.Strings(entries)
    if len(entries) > 0 {
        path := entries[len(entries)-1]
        ch, _ := sc.ReadFileChunks(path)
        fmt.Printf("Session first chunk tags: ")
        for i := 0; i < len(ch) && i < 12; i++ { fmt.Printf("%s ", ch[i].Tag) }
        fmt.Println()
        sess, err := sc.ParseSessionFile(path)
        if err != nil {
            log.Fatalf("parse session %s: %v", path, err)
        }
        fmt.Printf("Session: %s | entries: %d\n", path, len(sess.Entries))
        lim := 3
        if len(sess.Entries) < lim {
            lim = len(sess.Entries)
        }
        enc := json.NewEncoder(os.Stdout)
        enc.SetIndent("", "  ")
        for i := 0; i < lim; i++ {
            enc.Encode(sess.Entries[i])
        }
    }
}
