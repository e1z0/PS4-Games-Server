// SPDX-License-Identifier: GPL-2.0-only
package scanner

import (
	"context"
	"encoding/json"
	"path/filepath"
	"ps4server/internal/storage"
	"strings"
	"sync"
	"testing"
)

func TestExclusionsPersistAcrossScanRestartAndURLChange(t *testing.T) {
	c, _ := fixture(t)
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	v := e.Snapshot()[0]
	u := PackageURL(c, v)
	if err := e.SetFavorite(c, u, true); err != nil {
		t.Fatal(err)
	}
	before := e.Catalogs(c)
	if err := e.SetExcluded(c, u, true); err != nil {
		t.Fatal(err)
	}
	if before[v.Category].Data[u].Excluded {
		t.Fatal("mutated an existing HTTP catalog view")
	}
	if !e.Catalogs(c)[v.Category].Data[u].Excluded || !e.Catalogs(c)[v.Category].Data[u].Favorite {
		t.Fatal("favorite not published immediately")
	}
	if err := e.Run(context.Background(), c, true); err != nil {
		t.Fatal(err)
	}
	c.PublicURL = "http://other-nas:8080"
	e, err = New(c)
	if err != nil {
		t.Fatal(err)
	}
	u = PackageURL(c, v)
	if !e.Catalogs(c)[v.Category].Data[u].Excluded || !e.Catalogs(c)[v.Category].Data[u].Favorite {
		t.Fatal("favorite lost on rescan/restart/URL edit")
	}
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	var disk Catalog
	if err := storage.Read(filepath.Join(c.DataDir, "catalogs", v.Category+".json"), &disk); err != nil || !disk.Data[u].Excluded {
		t.Fatal("disk catalog lost favorite", err)
	}
	b, _ := json.Marshal(disk.Data[u])
	text := string(b)
	if !(strings.Index(text, `"size":`) < strings.Index(text, `"is_excluded":true`) && strings.Index(text, `"is_excluded":true`) < strings.Index(text, `"min_fw":`)) {
		t.Fatal("field type/order", text)
	}
	if err := e.SetExcluded(c, u, false); err != nil {
		t.Fatal(err)
	}
	e, err = New(c)
	if err != nil || e.Catalogs(c)[v.Category].Data[u].Excluded {
		t.Fatal("unfavorite not persisted", err)
	}
	if err := e.SetExcluded(c, "http://other-nas:8080/pkg/games/../config.json", true); err != ErrPackageNotFound {
		t.Fatal("unknown package accepted", err)
	}
	c.Mappings[0].Enabled = false
	if err := e.SetExcluded(c, u, true); err != ErrPackageNotFound {
		t.Fatal("disabled mapping accepted", err)
	}
}

func TestFavoritesExclusionsAndScansDoNotOverwriteEachOther(t *testing.T) {
	c, _ := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	v := e.Snapshot()[0]
	u := PackageURL(c, v)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for range 4 {
			if err := e.Run(context.Background(), c, false); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 4 {
			if err := e.SetFavorite(c, u, true); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 4 {
			if err := e.SetExcluded(c, u, true); err != nil {
				t.Error(err)
			}
		}
	}()
	wg.Wait()
	record := e.Catalogs(c)[v.Category].Data[u]
	if !record.Excluded || !record.Favorite || len(e.Catalogs(c)[v.Category].Data) != 1 {
		t.Fatal("preferences lost or excluded package removed", record)
	}
}
