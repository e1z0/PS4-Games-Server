// SPDX-License-Identifier: GPL-2.0-only
// Package ps4 reads bounded plaintext CNT/SFO metadata without decryption.
package ps4

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"ps4server/internal/storage"
	"regexp"
	"strconv"
	"strings"
)

type Entry struct {
	Offset    int64 `json:"offset"`
	Size      int64 `json:"size"`
	Encrypted bool  `json:"encrypted"`
}
type Package struct {
	TitleID   string   `json:"title_id"`
	ContentID string   `json:"content_id"`
	Title     string   `json:"title"`
	Version   string   `json:"version"`
	Category  string   `json:"category"`
	Firmware  *string  `json:"firmware"`
	Icon      Entry    `json:"icon"`
	Warnings  []string `json:"warnings,omitempty"`
}

func cstr(b []byte) string         { return strings.TrimSpace(string(bytes.SplitN(b, []byte{0}, 2)[0])) }
func bounds(o, n, size int64) bool { return o >= 0 && n >= 0 && o <= size && n <= size-o }
func read(r io.ReaderAt, o, n, size, limit int64) ([]byte, error) {
	if !bounds(o, n, size) || n > limit {
		return nil, errors.New("entry outside package or exceeds limit")
	}
	b := make([]byte, int(n))
	_, e := r.ReadAt(b, o)
	return b, e
}
func ParseSFO(b []byte) (map[string]any, error) {
	if len(b) < 20 || !bytes.Equal(b[:4], []byte{0, 0x50, 0x53, 0x46}) {
		return nil, errors.New("invalid SFO magic")
	}
	le := binary.LittleEndian
	k, d, n := int64(le.Uint32(b[8:])), int64(le.Uint32(b[12:])), int64(le.Uint32(b[16:]))
	size := int64(len(b))
	if n > 4096 || 20+n*16 > size || k < 20+n*16 || d < k || d > size {
		return nil, errors.New("invalid SFO table")
	}
	v := map[string]any{}
	for i := int64(0); i < n; i++ {
		p := 20 + i*16
		ko := k + int64(le.Uint16(b[p:]))
		format := le.Uint16(b[p+2:])
		sz := int64(le.Uint32(b[p+4:]))
		off := d + int64(le.Uint32(b[p+12:]))
		if ko >= d || !bounds(off, sz, size) {
			return nil, errors.New("SFO value outside table")
		}
		key := cstr(b[ko:d])
		switch format {
		case 0x0204, 0x0402:
			v[key] = cstr(b[off : off+sz])
		case 0x0404:
			if sz < 4 {
				return nil, errors.New("short SFO integer")
			}
			v[key] = le.Uint32(b[off:])
		}
	}
	return v, nil
}
func Firmware(v any) *string {
	n, ok := v.(uint32)
	if !ok || n == 0 {
		return nil
	}
	s := fmt.Sprintf("%08X", n)
	if strings.IndexAny(s[:4], "ABCDEF") >= 0 {
		return nil
	}
	major, _ := strconv.Atoi(s[:2])
	s = fmt.Sprintf("%d.%s", major, s[2:4])
	return &s
}

var titleID = regexp.MustCompile(`(?i)-(CUSA[0-9]{5})`)

func Inspect(path string) (Package, error) {
	f, e := os.Open(path)
	if e != nil {
		return Package{}, e
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return Package{}, e
	}
	return Parse(f, s.Size())
}
func Parse(r io.ReaderAt, size int64) (Package, error) {
	p := Package{Version: "0.00"}
	h, e := read(r, 0, 0xa0, size, 0xa0)
	if e != nil {
		return p, e
	}
	be := binary.BigEndian
	if be.Uint32(h) != 0x7f434e54 {
		return p, errors.New("not a PS4 CNT package")
	}
	n := int64(be.Uint32(h[0x10:]))
	dup := int64(be.Uint16(h[0x16:]))
	off := int64(be.Uint32(h[0x18:]))
	if n < 1 || n > 10000 || dup != 0 && dup != n || off < 0xa0 {
		return p, errors.New("invalid CNT table")
	}
	table, e := read(r, off, n*32, size, 320000)
	if e != nil {
		return p, e
	}
	entries := map[uint32]Entry{}
	for i := int64(0); i < n; i++ {
		b := table[i*32:]
		id := be.Uint32(b)
		v := Entry{int64(be.Uint32(b[16:])), int64(be.Uint32(b[20:])), be.Uint32(b[8:])&0x80000000 != 0}
		if _, ok := entries[id]; ok {
			return p, errors.New("duplicate CNT entry")
		}
		if !bounds(v.Offset, v.Size, size) {
			return p, errors.New("entry outside package")
		}
		entries[id] = v
	}
	p.ContentID = cstr(h[0x40:0x64])
	p.Icon = entries[0x1200]
	sfo := map[string]any{}
	if v, ok := entries[0x1000]; ok {
		if v.Encrypted {
			p.Warnings = append(p.Warnings, "param.sfo encrypted")
		} else {
			b, err := read(r, v.Offset, v.Size, size, 64<<20)
			if err == nil {
				sfo, err = ParseSFO(b)
			}
			if err != nil {
				p.Warnings = append(p.Warnings, "invalid param.sfo")
				sfo = map[string]any{}
			}
		}
	}
	str := func(k string) string { v, _ := sfo[k].(string); return v }
	if v := str("CONTENT_ID"); v != "" {
		p.ContentID = v
	}
	p.TitleID = str("TITLE_ID")
	if p.TitleID == "" {
		m := titleID.FindStringSubmatch(p.ContentID)
		if len(m) > 1 {
			p.TitleID = strings.ToUpper(m[1])
		}
	}
	p.Title = str("TITLE")
	p.Category = str("CATEGORY")
	for _, key := range []string{"VERSION", "APP_VER"} {
		v := str(key)
		a, _ := strconv.ParseFloat(v, 64)
		b, _ := strconv.ParseFloat(p.Version, 64)
		if a >= b && v != "" {
			p.Version = v
		}
	}
	p.Firmware = Firmware(sfo["SYSTEM_VER"])
	if p.Firmware == nil {
		p.Firmware = Firmware(sfo["SYSTEM_ROOT_VER"])
	}
	if p.Icon.Encrypted {
		p.Warnings = append(p.Warnings, "icon0.png encrypted")
	}
	return p, nil
}
func Classify(category, folder string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "gd", "gda", "gdo":
		return "game"
	case "gp", "gdp":
		return "update"
	case "ac":
		return "dlc"
	case "":
		switch strings.ToLower(folder) {
		case "game", "games":
			return "game"
		case "updates", "update", "patch", "patches":
			return "update"
		case "dlc", "addons", "add-on", "add-ons", "addon":
			return "dlc"
		}
	}
	return "unknown"
}
func Region(cid string) string {
	if len(cid) > 0 {
		switch cid[0] {
		case 'U':
			return "USA"
		case 'E':
			return "EUR"
		case 'J':
			return "JAP"
		case 'H':
			return "ASIA"
		}
	}
	return "UNK"
}
func ValidImage(b []byte) bool {
	if len(b) > 32<<20 {
		return false
	}
	c, _, e := image.DecodeConfig(bytes.NewReader(b))
	return e == nil && c.Width > 0 && c.Height > 0 && c.Width <= 16384 && c.Height <= 16384
}
func ValidImageFile(path string) bool {
	s, e := os.Stat(path)
	if e != nil || s.Size() > 32<<20 {
		return false
	}
	b, e := os.ReadFile(path)
	return e == nil && ValidImage(b)
}
func ExtractIcon(pkg string, entry Entry, dest string) (string, error) {
	if ValidImageFile(dest) {
		return "existing", nil
	}
	if entry.Encrypted {
		return "encrypted", nil
	}
	if entry.Size == 0 {
		return "missing", nil
	}
	f, e := os.Open(pkg)
	if e != nil {
		return "failed", e
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return "failed", e
	}
	b, e := read(f, entry.Offset, entry.Size, s.Size(), 32<<20)
	if e != nil {
		return "invalid", e
	}
	if !ValidImage(b) {
		return "invalid", nil
	}
	return "extracted", storage.Write(dest, b, 0600)
}
