// SPDX-License-Identifier: GPL-2.0-only
// Package testpkg constructs synthetic containers; no game content is included.
package testpkg

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"sort"
)

func Build(values map[string]any) []byte {
	defaults := map[string]any{"TITLE_ID": "CUSA12345", "CONTENT_ID": "UP0000-CUSA12345_00-SYNTHETIC000000", "TITLE": "Synthetic Game", "APP_VER": "01.23", "CATEGORY": "gd", "SYSTEM_VER": uint32(0x11020000)}
	for k, v := range values {
		defaults[k] = v
	}
	keys := []string{}
	for k := range defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	n := len(keys)
	keyTable := []byte{}
	data := []byte{}
	entries := make([]byte, n*16)
	le := binary.LittleEndian
	for i, k := range keys {
		p := entries[i*16:]
		le.PutUint16(p, uint16(len(keyTable)))
		keyTable = append(keyTable, append([]byte(k), 0)...)
		le.PutUint32(p[12:], uint32(len(data)))
		v := defaults[k]
		var b []byte
		switch x := v.(type) {
		case uint32:
			b = make([]byte, 4)
			le.PutUint32(b, x)
			le.PutUint16(p[2:], 0x0404)
		case string:
			b = append([]byte(x), 0)
			le.PutUint16(p[2:], 0x0204)
		}
		le.PutUint32(p[4:], uint32(len(b)))
		le.PutUint32(p[8:], uint32(len(b)))
		data = append(data, b...)
	}
	s := make([]byte, 20)
	copy(s, []byte{0, 0x50, 0x53, 0x46})
	le.PutUint32(s[4:], 0x101)
	le.PutUint32(s[8:], uint32(20+len(entries)))
	le.PutUint32(s[12:], uint32(20+len(entries)+len(keyTable)))
	le.PutUint32(s[16:], uint32(n))
	s = append(s, entries...)
	s = append(s, keyTable...)
	s = append(s, data...)
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.White)
	var icon bytes.Buffer
	png.Encode(&icon, im)
	out := make([]byte, 0x4000)
	be := binary.BigEndian
	be.PutUint32(out, 0x7f434e54)
	be.PutUint32(out[0x10:], 2)
	be.PutUint16(out[0x16:], 2)
	be.PutUint32(out[0x18:], 0x1000)
	copy(out[0x40:], []byte(defaults["CONTENT_ID"].(string)))
	for i, v := range []struct{ id, off, size uint32 }{{0x1000, 0x2100, uint32(len(s))}, {0x1200, 0x3000, uint32(icon.Len())}} {
		p := out[0x1000+i*32:]
		be.PutUint32(p, v.id)
		be.PutUint32(p[16:], v.off)
		be.PutUint32(p[20:], v.size)
	}
	copy(out[0x2100:], s)
	copy(out[0x3000:], icon.Bytes())
	return out
}
