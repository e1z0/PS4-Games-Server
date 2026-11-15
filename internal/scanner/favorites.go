// SPDX-License-Identifier: GPL-2.0-only
package scanner

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/storage"
)

var ErrPackageNotFound = errors.New("package is no longer in the library")

func favoriteKey(v Cached) string {
	b, _ := json.Marshal([]string{v.Mapping, v.Root, v.Relative})
	return string(b)
}

func applyFavorites(c config.Config, cats map[string]Catalog, entries []Cached, favorites map[string]bool) {
	for _, v := range entries {
		cat := cats[v.Category]
		url := PackageURL(c, v)
		record := cat.Data[url]
		record.Favorite = favorites[favoriteKey(v)]
		cat.Data[url] = record
	}
}

// SetFavorite serializes with scan publication so a running scan cannot
// overwrite a preference. Keys survive public URL changes and file updates.
func (e *Engine) SetFavorite(c config.Config, packageURL string, favorite bool) error {
	e.catalogMu.Lock()
	defer e.catalogMu.Unlock()
	entries := e.Snapshot()
	var selected *Cached
	for i := range entries {
		v := &entries[i]
		if PackageURL(c, *v) != packageURL {
			continue
		}
		for _, m := range c.Mappings {
			if m.Enabled && m.ID == v.Mapping && m.Path == v.Root {
				selected = v
				break
			}
		}
	}
	if selected == nil {
		return ErrPackageNotFound
	}
	cats := e.catalogsLocked(c)
	// Copy the affected catalog: existing HTTP requests may still encode the
	// previous immutable view while the preference is saved.
	updated := make(map[string]Catalog, len(cats))
	for category, cat := range cats {
		updated[category] = cat
	}
	cat := Catalog{Data: make(map[string]Record, len(cats[selected.Category].Data))}
	for url, record := range cats[selected.Category].Data {
		cat.Data[url] = record
	}
	record := cat.Data[packageURL]
	record.Favorite = favorite
	cat.Data[packageURL] = record
	updated[selected.Category] = cat
	next := make(map[string]bool, len(e.favorites)+1)
	for k, v := range e.favorites {
		next[k] = v
	}
	if favorite {
		next[favoriteKey(*selected)] = true
	} else {
		delete(next, favoriteKey(*selected))
	}
	if err := storage.JSON(filepath.Join(c.DataDir, "favorites.json"), next); err != nil {
		return err
	}
	e.favorites = next
	e.mu.Lock()
	e.revision++
	e.catalogRevision = e.revision
	e.published = updated
	e.mu.Unlock()
	if err := storage.JSON(filepath.Join(c.DataDir, "catalogs", selected.Category+".json"), cat); err != nil {
		// Preferences are authoritative and already committed; the live feed has
		// the saved value. A normal scan repairs this derived catalog file.
		e.Log("Favorite saved, but the generated catalog file could not be updated. Check storage permissions; the next scan will retry.")
	}
	return nil
}
