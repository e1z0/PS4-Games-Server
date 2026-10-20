// SPDX-License-Identifier: GPL-2.0-only
package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/ps4"
	"ps4server/internal/storage"
	"ps4server/internal/testpkg"
	"runtime"
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
	deadline := time.Now().Add(30 * time.Second)
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
	deadline = time.Now().Add(30 * time.Second)
	for e.Status().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !e.Status().Cancelled || string(e.MarshalSnapshot()) != before {
		t.Fatal("cancelled scan published", e.Status())
	}
}

func TestCustomCoverSharingAndCorruptImageRepair(t *testing.T) {
	c, _ := fixture(t)
	e, _ := New(c)
	e.Run(context.Background(), c, false)
	v := e.Snapshot()[0]
	own := ownCover(c, v)
	valid, err := os.ReadFile(own)
	if err != nil {
		t.Fatal(err)
	}
	if len(valid) < 34 {
		t.Fatal("fixture PNG too short")
	}
	os.WriteFile(own, valid[:33], 0600)
	if ps4.ValidImageFile(own) {
		t.Fatal("truncated PNG treated as valid")
	}
	e.Run(context.Background(), c, false)
	repaired, _ := os.ReadFile(own)
	if !bytes.Equal(valid, repaired) {
		t.Fatal("PNG not repaired")
	}
	im := image.NewRGBA(image.Rect(0, 0, 3, 3))
	im.Set(1, 1, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	png.Encode(&encoded, im)
	custom := encoded.Bytes()
	os.WriteFile(own, custom, 0600)
	e.Run(context.Background(), c, true)
	retained, _ := os.ReadFile(own)
	if !bytes.Equal(custom, retained) {
		t.Fatal("custom cover overwritten")
	}
	v.Category = "updates"
	v.Package.Category = "gp"
	if coverPath(c, v) != own {
		t.Fatal("base cover not shared with update")
	}
}

func TestMappingReplacedByFileRetainsCatalog(t *testing.T) {
	c, _ := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	before := string(e.MarshalSnapshot())
	if err := os.Rename(c.Mappings[0].Path, c.Mappings[0].Path+"-offline"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(c.Mappings[0].Path, []byte("not a directory"), 0600)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	if string(e.MarshalSnapshot()) != before || len(e.Status().Errors) == 0 {
		t.Fatal("unavailable mapping entries removed", e.Status())
	}
}
func TestFailedParseOfChangingFileRetainsPrevious(t *testing.T) {
	c, p := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	before := string(e.MarshalSnapshot())
	e.inspect = func(path string) (ps4.Package, error) {
		os.WriteFile(p, []byte("incomplete replacement"), 0600)
		return ps4.Inspect(path)
	}
	if err := e.Run(context.Background(), c, true); err != nil {
		t.Fatal(err)
	}
	if string(e.MarshalSnapshot()) != before || len(e.Status().Errors) == 0 {
		t.Fatal("changing invalid package removed old entry", e.Status())
	}
}
func TestUnavailableAliasDoesNotDuplicateOverlappingMapping(t *testing.T) {
	c, _ := fixture(t)
	real := c.Mappings[0].Path
	alias := real + "-alias"
	if err := os.Symlink(real, alias); err != nil {
		t.Skip(err)
	}
	c.Mappings = []config.Mapping{{ID: "a", Path: alias, Category: "games", Enabled: true}, {ID: "b", Path: real, Category: "games", Enabled: true}}
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	before := string(e.MarshalSnapshot())
	os.Remove(alias)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	if len(e.Snapshot()) != 1 || string(e.MarshalSnapshot()) != before {
		t.Fatal("retained alias duplicated or lost", e.Snapshot())
	}
}

func TestPublishedCatalogRefreshesWithoutMutatingPriorSnapshot(t *testing.T) {
	c, p := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	first := e.Catalogs(c)
	os.WriteFile(p, testpkg.Build(map[string]any{"TITLE": "Replacement title"}), 0600)
	if err := e.Run(context.Background(), c, true); err != nil {
		t.Fatal(err)
	}
	second := e.Catalogs(c)
	for url, game := range first["games"].Data {
		if game.Name != "Synthetic Game" || second["games"].Data[url].Name != "Replacement title" {
			t.Fatal("catalog snapshot stale or mutated")
		}
	}
	c.PublicURL = "http://edited:8080"
	third := e.Catalogs(c)
	for url := range third["games"].Data {
		if !strings.HasPrefix(url, c.PublicURL) {
			t.Fatal("public URL edit not applied", url)
		}
	}
}

func TestWriteFailureReportsErrorAndRetainsSnapshot(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix permission enforcement")
	}
	c, p := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	before := string(e.MarshalSnapshot())
	os.WriteFile(filepath.Join(filepath.Dir(p), "added.pkg"), testpkg.Build(map[string]any{"TITLE_ID": "CUSA54321"}), 0600)
	catalogDir := filepath.Join(c.DataDir, "catalogs")
	os.Chmod(catalogDir, 0500)
	defer os.Chmod(catalogDir, 0700)
	if err := e.Run(context.Background(), c, false); err == nil {
		t.Fatal("write failure not returned")
	}
	if len(e.Status().Errors) == 0 || string(e.MarshalSnapshot()) != before {
		t.Fatal("failed scan replaced snapshot or hid its error", e.Status())
	}
}

func TestMultipleFoldersSameCategoryAndIncrementalChanges(t *testing.T) {
	c, p := fixture(t)
	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "game.pkg"), testpkg.Build(map[string]any{"TITLE_ID": "CUSA54321"}), 0600)
	c.Mappings = append(c.Mappings, config.Mapping{ID: "other", Path: other, Category: "games", Enabled: true})
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	if len(e.Catalogs(c)["games"].Data) != 2 || e.Status().Inspected != 2 {
		t.Fatal("folders not combined", e.Status())
	}
	os.WriteFile(p, testpkg.Build(map[string]any{"TITLE": "Updated title"}), 0600)
	when := time.Now().Add(2 * time.Second)
	os.Chtimes(p, when, when)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	if e.Status().Inspected != 1 || e.Status().Reused != 1 {
		t.Fatal("changed package not detected", e.Status())
	}
	found := false
	for _, record := range e.Catalogs(c)["games"].Data {
		found = found || record.Name == "Updated title"
	}
	if !found {
		t.Fatal("changed metadata not published")
	}
}

func TestRestartCannotServeDisabledOrRepointedCachedRoots(t *testing.T) {
	c, _ := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	oldRoot := c.Mappings[0].Path
	c.Mappings[0].Path = t.TempDir()
	repointed, err := New(c)
	if err != nil || len(repointed.Snapshot()) != 0 {
		t.Fatal("repointed cached mapping survived restart", err)
	}
	c.Mappings[0].Path = oldRoot
	c.Mappings[0].Enabled = false
	disabled, err := New(c)
	if err != nil || len(disabled.Snapshot()) != 0 {
		t.Fatal("disabled cached mapping survived restart", err)
	}
	if err = disabled.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	var catalog Catalog
	if err = storage.Read(filepath.Join(c.DataDir, "catalogs", "games.json"), &catalog); err != nil || len(catalog.Data) != 0 || disabled.Status().Removed != 1 {
		t.Fatal("retired catalog not cleared", err, disabled.Status())
	}
}

type progressGate struct {
	entered chan struct{}
	finish  chan struct{}
}

func (g progressGate) Enrich(ctx context.Context, c config.Config, entries []Cached) (map[string]map[string]any, []string, error) {
	return g.EnrichWithProgress(ctx, c, entries, nil)
}
func (g progressGate) EnrichWithProgress(ctx context.Context, _ config.Config, entries []Cached, progress func(int, int, string)) (map[string]map[string]any, []string, error) {
	if progress != nil {
		progress(0, 1, "Waiting for optional game details")
	}
	close(g.entered)
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-g.finish:
		if progress != nil {
			progress(1, 1, "")
		}
		return map[string]map[string]any{entries[0].Package.TitleID: {"summary": "Fetched after local publication"}}, nil, nil
	}
}
func TestLocalCatalogIsPublishedBeforeOptionalMetadata(t *testing.T) {
	for _, cancelMetadata := range []bool{false, true} {
		t.Run(fmt.Sprint("cancel=", cancelMetadata), func(t *testing.T) {
			c, _ := fixture(t)
			e, err := New(c)
			if err != nil {
				t.Fatal(err)
			}
			gate := progressGate{make(chan struct{}), make(chan struct{})}
			e.Enricher = gate
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- e.Run(ctx, c, false) }()
			select {
			case <-gate.entered:
			case <-time.After(30 * time.Second):
				t.Fatal("metadata never started")
			}
			status := e.Status()
			if !status.Running || status.Phase != "metadata" || status.MetadataTotal != 1 || status.Current == "" {
				t.Fatal("missing live progress", status)
			}
			if len(e.Snapshot()) != 1 || len(e.Catalogs(c)["games"].Data) != 1 {
				t.Fatal("optional metadata hides local library")
			}
			restored, err := New(c)
			if err != nil || len(restored.Snapshot()) != 1 {
				t.Fatal("local results were not persisted", err)
			}
			if cancelMetadata {
				cancel()
			} else {
				close(gate.finish)
			}
			select {
			case err := <-done:
				if cancelMetadata && err == nil {
					t.Fatal("cancellation not reported")
				}
				if !cancelMetadata && err != nil {
					t.Fatal(err)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("scan never finished")
			}
			if len(e.Snapshot()) != 1 {
				t.Fatal("metadata cancellation removed published packages")
			}
			if cancelMetadata {
				if !e.Status().Cancelled || e.Status().Phase != "cancelled" {
					t.Fatal(e.Status())
				}
			} else {
				if e.Status().Phase != "complete" || e.Status().MetadataDone != 1 {
					t.Fatal(e.Status())
				}
				for _, record := range e.Catalogs(c)["games"].Data {
					if record.IGDB["summary"] != "Fetched after local publication" {
						t.Fatal("enrichment not added", record)
					}
				}
			}
		})
	}
}

func TestRoutineEncryptedSectionsAreInformational(t *testing.T) {
	c, _ := fixture(t)
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	e.inspect = func(path string) (ps4.Package, error) {
		p, err := ps4.Inspect(path)
		p.EncryptedEntries = 5
		p.Warnings = append(p.Warnings, "5 additional encrypted CNT entries", "icon0.png encrypted")
		return p, err
	}
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	if len(e.Status().Errors) != 1 || !strings.Contains(e.Status().Errors[0], "icon0.png encrypted") {
		t.Fatal("routine sections counted as problems or actionable warning lost", e.Status())
	}
	if !strings.Contains(strings.Join(e.Logs(), "\n"), "5 additional encrypted CNT entries") {
		t.Fatal("encrypted content was not reported")
	}
	if len(e.Snapshot()) != 1 || e.Snapshot()[0].Package.EncryptedEntries != 5 {
		t.Fatal("package details lost")
	}
}
