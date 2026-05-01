package main

import (
    "fmt"
    "log"
    sc "serato-songcompanion/seratosync"
)

func main() {
    ch, err := sc.ReadFileChunks("history.database")
    if err != nil { log.Fatal(err) }
    for _, c := range ch {
        if c.Tag == "oses" {
            adats := sc.ExtractADATPayloads(c.Data)
            if len(adats) == 0 { continue }
            fields, _ := sc.ParseADAT(adats[0])
            for _, f := range fields {
                s := sc.DecodeUTF16LE(f.Value)
                if len(s) > 0 {
                    fmt.Printf("id=%d len=%d s=%q\n", f.ID, len(f.Value), s)
                }
            }
            break
        }
    }
}

