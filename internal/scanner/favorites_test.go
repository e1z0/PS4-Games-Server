// SPDX-License-Identifier: GPL-2.0-only
package scanner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"ps4server/internal/storage"
	"strings"
	"sync"
	"testing"
)

func TestFavoritesPersistAcrossScanRestartAndURLChange(t *testing.T) {
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
	before := e.Catalogs(c)
	if err := e.SetFavorite(c, u, true); err != nil {
		t.Fatal(err)
	}
	if before[v.Category].Data[u].Favorite {
		t.Fatal("mutated an existing HTTP catalog view")
	}
	if !e.Catalogs(c)[v.Category].Data[u].Favorite {
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
	if !e.Catalogs(c)[v.Category].Data[u].Favorite {
		t.Fatal("favorite lost on rescan/restart/URL edit")
	}
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	var disk Catalog
	if err := storage.Read(filepath.Join(c.DataDir, "catalogs", v.Category+".json"), &disk); err != nil || !disk.Data[u].Favorite {
		t.Fatal("disk catalog lost favorite", err)
	}
	b, _ := json.Marshal(disk.Data[u])
	text := string(b)
	if !(strings.Index(text, `"size":`) < strings.Index(text, `"is_favorite":true`) && strings.Index(text, `"is_favorite":true`) < strings.Index(text, `"min_fw":`)) {
		t.Fatal("field type/order", text)
	}
	if err := e.SetFavorite(c, u, false); err != nil {
		t.Fatal(err)
	}
	e, err = New(c)
	if err != nil || e.Catalogs(c)[v.Category].Data[u].Favorite {
		t.Fatal("unfavorite not persisted", err)
	}
	if err := e.SetFavorite(c, "http://other-nas:8080/pkg/games/../config.json", true); err != ErrPackageNotFound {
		t.Fatal("unknown package accepted", err)
	}
	c.Mappings[0].Enabled = false
	if err := e.SetFavorite(c, u, true); err != ErrPackageNotFound {
		t.Fatal("disabled mapping accepted", err)
	}
}

func TestFavoriteConcurrentPublicationAndFailedSave(t *testing.T) {
	c, _ := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	v := e.Snapshot()[0]
	u := PackageURL(c, v)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 5 {
			if err := e.Run(context.Background(), c, false); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 5 {
			if err := e.SetFavorite(c, u, true); err != nil {
				t.Error(err)
			}
			_ = e.Catalogs(c)
		}
	}()
	wg.Wait()
	if !e.Catalogs(c)[v.Category].Data[u].Favorite {
		t.Fatal("scan overwrote favorite")
	}
	if err := os.Remove(filepath.Join(c.DataDir, "favorites.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(c.DataDir, "favorites.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := e.SetFavorite(c, u, false); err == nil {
		t.Fatal("write failure ignored")
	}
	if !e.Catalogs(c)[v.Category].Data[u].Favorite {
		t.Fatal("failed write changed published state")
	}
}

func TestFavoriteRetainsAuthoritativeStateIfDerivedCatalogFails(t *testing.T) {
	c, _ := fixture(t)
	e, _ := New(c)
	if err := e.Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	v := e.Snapshot()[0]
	u := PackageURL(c, v)
	_ = e.Catalogs(c)
	catalog := filepath.Join(c.DataDir, "catalogs", v.Category+".json")
	if err := os.Remove(catalog); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(catalog, 0700); err != nil {
		t.Fatal(err)
	}
	if err := e.SetFavorite(c, u, true); err != nil {
		t.Fatal("preference should still save", err)
	}
	restarted, err := New(c)
	if err != nil || !restarted.Catalogs(c)[v.Category].Data[u].Favorite {
		t.Fatal("authoritative preference not saved", err)
	}
	if !strings.Contains(strings.Join(e.Logs(), "\n"), "generated catalog file could not be updated") {
		t.Fatal("derived catalog failure not logged")
	}
}
