// SPDX-License-Identifier: GPL-2.0-only
package ps4

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImageValidationReuseAndRepair(t *testing.T) {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	path := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	cache := ImageCache{}
	if !cache.Valid("shot.png", path) || len(cache) != 1 {
		t.Fatal("valid image not recorded")
	}
	encoded, _ := json.Marshal(cache)
	var restored ImageCache
	json.Unmarshal(encoded, &restored)
	if !restored.Valid("shot.png", path) {
		t.Fatal("persisted validation not reusable")
	}
	info, _ := os.Stat(path)
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), b.Len()), 0600); err != nil {
		t.Fatal(err)
	}
	// A changed file with the same length must not inherit the valid stamp.
	if err := os.Chtimes(path, info.ModTime().Add(time.Second), info.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if restored.Valid("shot.png", path) || len(restored) != 0 {
		t.Fatal("damaged image reused")
	}
	os.WriteFile(path, b.Bytes(), 0600)
	if !restored.Valid("shot.png", path) {
		t.Fatal("repaired image not revalidated")
	}
	os.Remove(path)
	if restored.Valid("shot.png", path) || len(restored) != 0 {
		t.Fatal("missing image reused")
	}
}
