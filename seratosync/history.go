package seratosync

import (
    "encoding/binary"
    "time"
)

type HistoryDatabase struct {
    Version string
    Sessions []HistorySession
}

type HistorySession struct {
    GUID        string
    DateText    string
    Device      string
    TrackCount  int
    RawFields   []TLV
}

func ParseHistoryDatabase(path string) (*HistoryDatabase, error) {
    chunks, err := ReadFileChunks(path)
    if err != nil {
        return nil, err
    }
    out := &HistoryDatabase{}
    for i := 0; i < len(chunks); i++ {
        ch := chunks[i]
        if ch.Tag == "vrsn" {
            out.Version = DecodeUTF16LE(ch.Data)
            continue
        }
        if ch.Tag == "oses" {
            var adat []byte
            payloads := ExtractADATPayloads(ch.Data)
            if len(payloads) > 0 {
                adat = payloads[0]
            }
            hs := parseHistorySessionPair(ch, adat)
            out.Sessions = append(out.Sessions, hs)
        }
    }
    return out, nil
}

func parseHistorySessionPair(desc Chunk, adat []byte) HistorySession {
    var hs HistorySession
    if len(adat) > 0 {
        fields, _ := ParseADAT(adat)
        hs.RawFields = fields
        if s, ok := FirstStringMatching(fields, func(s string) bool { return reGUID.MatchString(s) }); ok {
            hs.GUID = s
        }
        if s, ok := FirstStringMatching(fields, func(s string) bool { return s == "Offline Player" }); ok {
            hs.Device = s
        } else if s, ok := FirstStringMatching(fields, func(s string) bool { return len(s) > 0 && len(s) < 64 && (s == "DJ" || s == "SL" || s == "Serato DJ" || s == "Serato Scratch LIVE" || s == "Offline Player") }); ok {
            if hs.Device == "" {
                hs.Device = s
            }
        }
        if s, ok := FirstStringMatching(fields, func(s string) bool { return reDateMDY.MatchString(s) }); ok {
            hs.DateText = s
        }
        // Track count: find a 4-byte integer with small plausible value near 1..1000
        for _, f := range fields {
            if len(f.Value) == 4 {
                v := int(binary.LittleEndian.Uint32(f.Value))
                if v > 0 && v < 5000 {
                    if v > hs.TrackCount {
                        hs.TrackCount = v
                    }
                }
            }
        }
        if hs.GUID == "" || hs.Device == "" || hs.DateText == "" || hs.TrackCount == 0 {
            strs := ExtractUTF16Strings(adat, 2)
            for _, s := range strs {
                if hs.GUID == "" && reGUID.MatchString(s) { hs.GUID = s }
                if hs.Device == "" && (s == "Offline Player" || s == "Serato DJ" || s == "DJ") { hs.Device = s }
                if hs.DateText == "" && reDateMDY.MatchString(s) { hs.DateText = s }
            }
        }
    }
    return hs
}

// Utility if we later discover the 8-byte timestamps format.
func guessTimeFrom64(u uint64) (time.Time, bool) {
    if u == 0 {
        return time.Time{}, false
    }
    // Try Unix seconds
    if u > 946684800 && u < 4102444800 { // ~2000-2100
        return time.Unix(int64(u), 0), true
    }
    // Try milliseconds
    if u > 946684800000 && u < 4102444800000 {
        sec := int64(u / 1000)
        nsec := int64(u%1000) * int64(time.Millisecond)
        return time.Unix(sec, nsec), true
    }
    return time.Time{}, false
}
