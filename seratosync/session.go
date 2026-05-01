package seratosync

import (
    "encoding/binary"
    "path/filepath"
    "regexp"
    "strconv"
    "strings"
)

type Session struct {
    Version string
    Entries []SessionEntry
}

type SessionEntry struct {
    FilePath string
    Title    string
    Artist   string
    Genres   []string
    Key      string
    BPM      float64
    Energy   int
    RawFields []TLV
}

var reExt = regexp.MustCompile(`\.(mp3|m4a|wav|flac|aif|aiff)$`)

func ParseSessionFile(path string) (*Session, error) {
    chunks, err := ReadFileChunks(path)
    if err != nil {
        return nil, err
    }
    sess := &Session{}
    for i := 0; i < len(chunks); i++ {
        ch := chunks[i]
        if ch.Tag == "vrsn" {
            sess.Version = DecodeUTF16LE(ch.Data)
            continue
        }
        if ch.Tag == "oent" {
            var adat []byte
            if i+1 < len(chunks) && chunks[i+1].Tag == "adat" {
                adat = chunks[i+1].Data
                i++
            }
            e := parseSessionEntryPair(ch, adat)
            if e.FilePath != "" || e.Title != "" {
                sess.Entries = append(sess.Entries, e)
            }
        }
    }
    return sess, nil
}

func parseSessionEntryPair(desc Chunk, adat []byte) SessionEntry {
    var se SessionEntry
    if len(adat) > 0 {
        fields, _ := ParseADAT(adat)
        se.RawFields = fields
        if se.FilePath == "" {
            if s, ok := FirstStringMatching(fields, func(s string) bool {
                return rePath.MatchString(s) && reExt.MatchString(strings.ToLower(s))
            }); ok {
                se.FilePath = filepath.Clean(s)
            }
        }
        if se.Title == "" {
            if s, ok := FirstStringMatching(fields, func(s string) bool {
                if s == "" || strings.Contains(s, `:\\`) || strings.Contains(s, `\\`) {
                    return false
                }
                if reGenres.MatchString(s) {
                    return false
                }
                if reGUID.MatchString(s) {
                    return false
                }
                return len(s) > 0 && len(s) < 256
            }); ok {
                se.Title = s
            }
        }
        if se.Artist == "" {
            if s, ok := FirstStringMatching(fields, func(s string) bool {
                if s == se.Title || s == se.FilePath {
                    return false
                }
                if reGenres.MatchString(s) {
                    return false
                }
                return len(s) > 0 && len(s) < 128 && !strings.Contains(s, `:\\`)
            }); ok {
                se.Artist = s
            }
        }
        if len(se.Genres) == 0 {
            if s, ok := FirstStringMatching(fields, func(s string) bool { return reGenres.MatchString(s) }); ok {
                parts := strings.Split(s, ",")
                for i := range parts {
                    parts[i] = strings.TrimSpace(parts[i])
                }
                se.Genres = parts
            }
        }
        if se.Key == "" || se.BPM == 0 {
            if s, ok := FirstStringMatching(fields, func(s string) bool { return reKeyTempo.MatchString(s) }); ok {
                ks, bpms := splitKeyTempo(s)
                se.Key = ks
                se.BPM = bpms
            }
        }
        if se.Energy == 0 {
            if s, ok := FirstStringMatching(fields, func(s string) bool { return strings.HasPrefix(s, "Energy ") }); ok {
                n := strings.TrimPrefix(s, "Energy ")
                if v, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
                    se.Energy = v
                }
            }
        }
        if se.FilePath == "" || se.Title == "" || se.Artist == "" || (se.Key == "" && se.BPM == 0) || se.Energy == 0 || len(se.Genres) == 0 {
            strs := ExtractUTF16Strings(adat, 2)
            for _, s := range strs {
                if se.FilePath == "" && rePath.MatchString(s) && reExt.MatchString(strings.ToLower(s)) { se.FilePath = filepath.Clean(s) }
                if len(se.Genres) == 0 && reGenres.MatchString(s) { 
                    parts := strings.Split(s, ","); for i := range parts { parts[i] = strings.TrimSpace(parts[i]) }; se.Genres = parts }
                if (se.Key == "" || se.BPM == 0) && reKeyTempo.MatchString(s) { k, b := splitKeyTempo(s); se.Key, se.BPM = k, b }
                if se.Energy == 0 && strings.HasPrefix(s, "Energy ") { n := strings.TrimPrefix(s, "Energy "); if v, err := strconv.Atoi(strings.TrimSpace(n)); err == nil { se.Energy = v } }
            }
            // Title/Artist heuristic: pick two distinct non-path strings near genres/key if still empty
            if se.Title == "" || se.Artist == "" {
                cand := []string{}
                for _, s := range strs {
                    if s == "" || rePath.MatchString(s) || reGUID.MatchString(s) || reGenres.MatchString(s) || reKeyTempo.MatchString(s) { continue }
                    if strings.Contains(s, ":\\") || strings.Contains(s, "\\") { continue }
                    if len(s) > 0 && len(s) <= 128 { cand = append(cand, s) }
                }
                if len(cand) > 0 && se.Title == "" { se.Title = cand[0] }
                if len(cand) > 1 && se.Artist == "" { se.Artist = cand[1] }
            }
        }
    }
    return se
}

func splitKeyTempo(s string) (string, float64) {
    parts := strings.Split(s, "-")
    if len(parts) != 2 {
        return strings.TrimSpace(s), 0
    }
    key := strings.TrimSpace(parts[0])
    tempo := strings.TrimSpace(parts[1])
    tempo = strings.Fields(tempo)[0]
    f, _ := strconv.ParseFloat(tempo, 64)
    return key, f
}

func GetUint32(v []byte) (uint32, bool) {
    if len(v) != 4 {
        return 0, false
    }
    return binary.LittleEndian.Uint32(v), true
}
