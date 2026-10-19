// SPDX-License-Identifier: GPL-2.0-only
package ps4

import (
	"bytes"
	"encoding/binary"
	"ps4server/internal/testpkg"
	"testing"
)

func TestMetadata(t *testing.T) {
	b := testpkg.Build(nil)
	p, e := Parse(bytes.NewReader(b), int64(len(b)))
	if e != nil || p.TitleID != "CUSA12345" || p.Version != "01.23" || p.Firmware == nil || *p.Firmware != "11.02" {
		t.Fatal(p, e)
	}
	b = testpkg.Build(map[string]any{"TITLE_ID": "BREW00001", "CONTENT_ID": "", "SYSTEM_VER": uint32(0), "SYSTEM_ROOT_VER": uint32(0x07550000), "CATEGORY": "xx"})
	p, e = Parse(bytes.NewReader(b), int64(len(b)))
	if e != nil || p.TitleID != "BREW00001" || *p.Firmware != "7.55" || Classify(p.Category, "games") != "unknown" {
		t.Fatal(p, e)
	}
}
func TestBoundsAndEncryption(t *testing.T) {
	for _, mut := range []func([]byte){func(b []byte) { b[0] = 0 }, func(b []byte) { binary.BigEndian.PutUint32(b[0x1010:], 0xfffffff0) }, func(b []byte) { binary.BigEndian.PutUint32(b[0x10:], 10001) }, func(b []byte) { binary.BigEndian.PutUint32(b[0x1020:], 0x1000) }} {
		b := testpkg.Build(nil)
		mut(b)
		if _, e := Parse(bytes.NewReader(b), int64(len(b))); e == nil {
			t.Fatal("malformed accepted")
		}
	}
	for n := 0; n < 0xa0; n++ {
		if _, e := Parse(bytes.NewReader(make([]byte, n)), int64(n)); e == nil {
			t.Fatal(n)
		}
	}
	b := testpkg.Build(nil)
	binary.BigEndian.PutUint32(b[0x1008:], 0x80000000)
	binary.BigEndian.PutUint32(b[0x1028:], 0x80000000)
	p, e := Parse(bytes.NewReader(b), int64(len(b)))
	if e != nil || !p.Icon.Encrypted || len(p.Warnings) != 2 {
		t.Fatal(p, e)
	}
	binary.BigEndian.PutUint32(b[0x1014:], 65<<20)
	if _, e := Parse(bytes.NewReader(b), int64(len(b))); e == nil {
		t.Fatal("oversize accepted")
	}
}
func TestFirmwareAndTypes(t *testing.T) {
	if Firmware(uint32(0x0b000000)) != nil {
		t.Fatal("invalid BCD")
	}
	for k, v := range map[string]string{"gd": "game", "gda": "game", "gdo": "game", "gp": "update", "gdp": "update", "ac": "dlc", "other": "unknown"} {
		if Classify(k, "games") != v {
			t.Fatal(k)
		}
	}
}
func FuzzParse(f *testing.F) {
	f.Add(testpkg.Build(nil))
	f.Fuzz(func(t *testing.T, b []byte) { Parse(bytes.NewReader(b), int64(len(b))); ParseSFO(b) })
}
