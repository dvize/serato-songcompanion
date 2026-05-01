package seratosync

import (
    "bytes"
    "encoding/binary"
    "errors"
    "io"
    "os"
    "regexp"
    "strings"
    "unicode/utf16"
)


type Chunk struct {
    Tag     string
    Data    []byte
    Children []Chunk
}

func ReadFileChunks(path string) ([]Chunk, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    return readChunks(f, -1)
}

func readChunks(r io.Reader, limit int64) ([]Chunk, error) {
    var chunks []Chunk
    var rr io.Reader = r
    var lr *io.LimitedReader
    if limit >= 0 {
        lr = &io.LimitedReader{R: r, N: limit}
        rr = lr
    }
    for {
        header := make([]byte, 8)
        var n int
        var err error
        if limit >= 0 {
            if lr.N <= 0 {
                break
            }
        }
        n, err = io.ReadFull(rr, header)
        if err != nil {
            if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
                break
            }
            return nil, err
        }
        if n != 8 {
            break
        }
        tag := string(header[:4])
        size := int64(binary.BigEndian.Uint32(header[4:]))
        if size < 0 {
            return nil, errors.New("negative chunk size")
        }
        data := make([]byte, size)
        if _, err := io.ReadFull(rr, data); err != nil {
            return nil, err
        }
        children := parseChildrenIfChunked(data)
        chunks = append(chunks, Chunk{Tag: tag, Data: data, Children: children})
    }
    return chunks, nil
}

func parseChildrenIfChunked(b []byte) []Chunk {
    if len(b) < 8 {
        return nil
    }
    r := bytes.NewReader(b)
    var children []Chunk
    for r.Len() >= 8 {
        hdr := make([]byte, 8)
        if _, err := io.ReadFull(r, hdr); err != nil {
            return nil
        }
        if !isASCII(hdr[:4]) {
            return nil
        }
        sz := int64(binary.BigEndian.Uint32(hdr[4:]))
        if sz < 0 || sz > int64(r.Len()) {
            return nil
        }
        data := make([]byte, sz)
        if _, err := io.ReadFull(r, data); err != nil {
            return nil
        }
        subChildren := parseChildrenIfChunked(data)
        children = append(children, Chunk{Tag: string(hdr[:4]), Data: data, Children: subChildren})
    }
    // If we consumed all bytes, accept; otherwise treat as non-chunked payload
    if r.Len() == 0 {
        return children
    }
    return nil
}

func isASCII(p []byte) bool {
    for _, c := range p {
        if c < 0x20 || c > 0x7E { // conservative
            return false
        }
    }
    return true
}

type TLV struct {
    ID    uint32
    Value []byte
}

func ParseADAT(data []byte) ([]TLV, error) {
    var fields []TLV
    pos := 0
    for pos+8 <= len(data) {
        id := binary.LittleEndian.Uint32(data[pos : pos+4])
        ln := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
        if ln <= uint32(len(data)-(pos+8)) && ln < 1<<24 {
            v := make([]byte, ln)
            copy(v, data[pos+8:pos+8+int(ln)])
            fields = append(fields, TLV{ID: id, Value: v})
            pos += 8 + int(ln)
        } else {
            pos++
        }
    }
    return fields, nil
}

func DecodeUTF16LE(data []byte) string {
    if len(data) == 0 {
        return ""
    }
    if len(data)%2 == 1 {
        data = data[:len(data)-1]
    }
    u16 := make([]uint16, 0, len(data)/2)
    for i := 0; i+1 < len(data); i += 2 {
        u16 = append(u16, binary.LittleEndian.Uint16(data[i:i+2]))
    }
    // Trim trailing NULs
    for len(u16) > 0 && u16[len(u16)-1] == 0 {
        u16 = u16[:len(u16)-1]
    }
    runes := utf16.Decode(u16)
    s := string(runes)
    return strings.TrimRight(s, "\u0000")
}

func DecodeUTF16BE(data []byte) string {
    if len(data) == 0 {
        return ""
    }
    if len(data)%2 == 1 {
        data = data[:len(data)-1]
    }
    u16 := make([]uint16, 0, len(data)/2)
    for i := 0; i+1 < len(data); i += 2 {
        u16 = append(u16, binary.BigEndian.Uint16(data[i:i+2]))
    }
    // Trim trailing NULs
    for len(u16) > 0 && u16[len(u16)-1] == 0 {
        u16 = u16[:len(u16)-1]
    }
    runes := utf16.Decode(u16)
    return strings.TrimRight(string(runes), "\u0000")
}

var (
    reGUID     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
    rePath     = regexp.MustCompile(`^[A-Za-z]:\\`)
    reGenres   = regexp.MustCompile(`,`)
    reKeyTempo = regexp.MustCompile(`^\d{1,2}[AB]\s*-\s*\d+(?:\.\d+)?$`)
    reDateMDY  = regexp.MustCompile(`^\d{1,2}/\d{1,2}/\d{2,4}$`)
)

func FirstStringMatching(fields []TLV, fn func(string) bool) (string, bool) {
    for _, f := range fields {
        s := DecodeUTF16LE(f.Value)
        if s == "" {
            continue
        }
        if fn(s) {
            return s, true
        }
    }
    return "", false
}

func AllStrings(fields []TLV) []string {
    var out []string
    for _, f := range fields {
        s := DecodeUTF16LE(f.Value)
        if strings.TrimSpace(s) != "" {
            out = append(out, s)
        }
    }
    return out
}

func ExtractADATPayloads(b []byte) [][]byte {
    var out [][]byte
    for i := 0; i+8 <= len(b); i++ {
        if string(b[i:i+4]) != "adat" {
            continue
        }
        ln := int(binary.BigEndian.Uint32(b[i+4 : i+8]))
        if ln < 0 || i+8+ln > len(b) {
            continue
        }
        out = append(out, b[i+8:i+8+ln])
        i += 8 + ln - 1
    }
    return out
}

func ExtractUTF16Strings(b []byte, minRunes int) []string {
    var out []string
    i := 0
    for i+1 < len(b) {
        start := i
        var u16 []uint16
        for i+1 < len(b) {
            v := binary.LittleEndian.Uint16(b[i : i+2])
            if v == 0 {
                // end of a string
                if len(u16) >= minRunes {
                    out = append(out, string(utf16.Decode(u16)))
                }
                i += 2
                break
            }
            // accept printable range
            if v < 0x20 || v > 0x7E {
                if len(u16) >= minRunes {
                    out = append(out, string(utf16.Decode(u16)))
                }
                i += 2
                u16 = nil
                break
            }
            u16 = append(u16, v)
            i += 2
        }
        if start == i {
            i++
        }
    }
    // de-dup
    seen := map[string]bool{}
    uniq := make([]string, 0, len(out))
    for _, s := range out {
        s = strings.TrimSpace(s)
        if s == "" || seen[s] {
            continue
        }
        seen[s] = true
        uniq = append(uniq, s)
    }
    return uniq
}
