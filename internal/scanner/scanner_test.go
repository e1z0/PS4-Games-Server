// SPDX-License-Identifier: GPL-2.0-only
package scanner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/ps4"
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

func TestCatalogGolden(t *testing.T) {
	c := config.Defaults()
	c.PublicURL = "http://nas:8080"
	c.DataDir = t.TempDir()
	c.Mappings = []config.Mapping{{ID: "main", Category: "games", Enabled: true}}
	fw := "11.02"
	v := Cached{Mapping: "main", Relative: "nested 猫/game #1.pkg", Category: "games", Size: 5368709120, Release: "10-19-2026", Package: ps4.Package{TitleID: "CUSA12345", ContentID: "UP0000-CUSA12345_00-SYNTHETIC000000", Title: "Synthetic Game", Category: "gd", Version: "01.23", Firmware: &fw}}
	b, err := json.MarshalIndent(Catalogs(c, []Cached{v}, nil)["games"], "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	gold, e := os.ReadFile("testdata/catalog.golden.json")
	if e != nil {
		t.Fatal(e)
	}
	if string(append(b, '\n')) != string(gold) {
		t.Fatalf("catalog mismatch:\n%s", b)
	}
}

func TestChangingFileAndActiveCancellation(t *testing.T) {
	c, p := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	before := string(e.MarshalSnapshot())
	entered, release := make(chan struct{}), make(chan struct{})
	e.inspect = func(path string) (ps4.Package, error) {
		parsed, err := ps4.Inspect(path)
		close(entered)
		<-release
		return parsed, err
	}
	if err := e.Start(context.Background(), c, true); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := e.Start(context.Background(), c, false); err == nil {
		t.Fatal("active overlap allowed")
	}
	os.WriteFile(p, testpkg.Build(map[string]any{"TITLE": "Changed during scan"}), 0600)
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for e.Status().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if string(e.MarshalSnapshot()) != before || len(e.Status().Errors) == 0 {
		t.Fatal("unstable package published", e.Status())
	}
	entered, release = make(chan struct{}), make(chan struct{})
	e.inspect = func(path string) (ps4.Package, error) { close(entered); <-release; return ps4.Inspect(path) }
	if err := e.Start(context.Background(), c, true); err != nil {
		t.Fatal(err)
	}
	<-entered
	e.Cancel()
	close(release)
	deadline = time.Now().Add(3 * time.Second)
	for e.Status().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !e.Status().Cancelled || string(e.MarshalSnapshot()) != before {
		t.Fatal("cancelled scan published", e.Status())
	}
}
