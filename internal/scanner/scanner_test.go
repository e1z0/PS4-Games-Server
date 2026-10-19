// SPDX-License-Identifier: GPL-2.0-only
package scanner

import (
	"context"
	"os"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/testpkg"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "packages")
	os.MkdirAll(filepath.Join(root, "nested [猫]"), 0700)
	p := filepath.Join(root, "nested [猫]", "game #1.pkg")
	os.WriteFile(p, testpkg.Build(nil), 0600)
	c := config.Defaults()
	c.PublicURL = "http://nas:8080"
	c.DataDir = filepath.Join(dir, "data")
	c.Mappings = []config.Mapping{{ID: "games", Path: root, Category: "games", Enabled: true}}
	return c, p
}
func TestIncrementalRestartRemovalRepair(t *testing.T) {
	c, p := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	if e.Status().Inspected != 1 {
		t.Fatal(e.Status())
	}
	cat := filepath.Join(c.DataDir, "catalogs", "games.json")
	old := time.Unix(1000000, 0)
	os.Chtimes(cat, old, old)
	e, _ = New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	s, _ := os.Stat(cat)
	if e.Status().Reused != 1 || !s.ModTime().Equal(old) {
		t.Fatal(e.Status(), s.ModTime())
	}
	v := e.Snapshot()[0]
	cover := coverPath(c, v)
	os.WriteFile(cover, nil, 0600)
	e.Run(context.Background(), c, false)
	s, _ = os.Stat(cover)
	if s.Size() == 0 {
		t.Fatal("cover not repaired")
	}
	url := PackageURL(c, v)
	if !strings.Contains(url, "nested%20%5B%E7%8C%AB%5D/game%20%231.pkg") {
		t.Fatal(url)
	}
	os.Remove(p)
	e.Run(context.Background(), c, false)
	if len(e.Snapshot()) != 0 || e.Status().Removed != 1 {
		t.Fatal(e.Status())
	}
}
func TestUnavailableOverlapAndCancel(t *testing.T) {
	c, _ := fixture(t)
	c.Mappings = append(c.Mappings, config.Mapping{ID: "other", Path: c.Mappings[0].Path, Category: "games", Enabled: true})
	e, _ := New(c)
	e.Run(context.Background(), c, false)
	if len(e.Snapshot()) != 1 {
		t.Fatal("not deduplicated")
	}
	os.Rename(c.Mappings[0].Path, c.Mappings[0].Path+"-offline")
	e.Run(context.Background(), c, false)
	if len(e.Snapshot()) != 1 || len(e.Status().Errors) == 0 {
		t.Fatal("unavailable mapping lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e.Run(ctx, c, false) == nil || !e.Status().Cancelled {
		t.Fatal("cancellation not reported")
	}
	if _, err := e.begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.Start(context.Background(), c, false) == nil {
		t.Fatal("overlap allowed")
	}
	e.Cancel()
}
func TestLargeFileAndDLC(t *testing.T) {
	c, p := fixture(t)
	f, err := os.OpenFile(p, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(5 << 30); err != nil {
		t.Skip(err)
	}
	f.Close()
	e, _ := New(c)
	if err = e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	var rec Record
	for _, r := range e.Catalogs(c)["games"].Data {
		rec = r
	}
	if rec.Size != "5368709120" {
		t.Fatal(rec.Size)
	}
	v := e.Snapshot()[0]
	v.Package.Category = "ac"
	v.Package.ContentID = "UP0000-CUSA12345_00-ONE"
	a := CoverKey(v)
	v.Package.ContentID = "UP0000-CUSA12345_00-TWO"
	if a == CoverKey(v) {
		t.Fatal("DLC cover collision")
	}
}
