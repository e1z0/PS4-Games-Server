// SPDX-License-Identifier: GPL-2.0-only
package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/ps4"
	"ps4server/internal/scanner"
	"ps4server/internal/storage"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheRenewalOfflineOverridesAndRepair(t *testing.T) {
	var tokens, queries, images atomic.Int32
	var offline atomic.Bool
	var buf bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.White)
	jpeg.Encode(&buf, im, nil)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokens.Add(1)
			json.NewEncoder(w).Encode(Object{"access_token": "token", "expires_in": 3600})
		case "/games":
			n := queries.Add(1)
			if offline.Load() {
				w.WriteHeader(503)
				return
			}
			if n == 1 {
				w.WriteHeader(401)
				return
			}
			b, _ := io.ReadAll(r.Body)
			if strings.Contains(string(b), "search") {
				json.NewEncoder(w).Encode([]Object{{"id": 123, "name": "Synthetic Game"}})
			} else {
				json.NewEncoder(w).Encode([]Object{{"id": 123, "name": "Synthetic Game", "summary": "Rich summary", "screenshots": []Object{{"image_id": "shot", "width": 2, "height": 2}}}})
			}
		default:
			images.Add(1)
			w.Write(buf.Bytes())
		}
	}))
	defer ts.Close()
	s := New()
	s.TokenURL = ts.URL + "/token"
	s.GamesURL = ts.URL + "/games"
	s.ImageBase = ts.URL + "/images/"
	now := time.Now()
	s.Now = func() time.Time { return now }
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.PublicURL = "http://nas:8080"
	c.IGDB.Enabled = true
	c.IGDB.ClientID = "id"
	c.IGDB.ClientSecret = "secret"
	c.IGDB.IntervalMS = 0
	entries := []scanner.Cached{{Category: "games", Package: ps4.Package{TitleID: "CUSA12345", Title: "Synthetic Game", Category: "gd"}}, {Category: "updates", Package: ps4.Package{TitleID: "CUSA12345", Title: "Update", Category: "gp"}}}
	out, warn, e := s.Enrich(context.Background(), c, entries)
	if e != nil || len(warn) > 0 || out["CUSA12345"]["summary"] != "Rich summary" || tokens.Load() != 2 || images.Load() != 1 {
		t.Fatal(out, warn, e, tokens.Load(), queries.Load())
	}
	q := queries.Load()
	s.Enrich(context.Background(), c, entries)
	if queries.Load() != q || images.Load() != 1 {
		t.Fatal("cache not used")
	}
	screenshot := filepath.Join(c.CoversDir(), "igdb/screenshots/CUSA12345/shot.jpg")
	os.WriteFile(screenshot, nil, 0600)
	s.Enrich(context.Background(), c, entries)
	if images.Load() != 2 {
		t.Fatal("screenshot not repaired")
	}
	now = now.Add(31 * 24 * time.Hour)
	offline.Store(true)
	out, warn, e = s.Enrich(context.Background(), c, entries)
	if e != nil || len(warn) == 0 || out["CUSA12345"]["summary"] != "Rich summary" {
		t.Fatal("offline lost metadata", e, warn)
	}
	cache := Cache{}
	storage.Read(filepath.Join(c.DataDir, "metadata-cache.json"), &cache)
	if !cache.Titles["CUSA12345"].Found {
		t.Fatal("network error cached negatively")
	}
	storage.JSON(filepath.Join(c.DataDir, "metadata-overrides.json"), map[string]Object{"CUSA12345": {"summary": "Manual"}})
	out, _, _ = s.Enrich(context.Background(), c, entries)
	if out["CUSA12345"]["summary"] != "Manual" {
		t.Fatal("override missing")
	}
}
func TestNegativeAndThrottle(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			json.NewEncoder(w).Encode(Object{"access_token": "t", "expires_in": 3600})
			return
		}
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		json.NewEncoder(w).Encode([]Object{{"id": 1, "name": "Unrelated racing game"}})
	}))
	defer ts.Close()
	s := New()
	s.TokenURL = ts.URL + "/token"
	s.GamesURL = ts.URL + "/games"
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.IGDB.Enabled = true
	c.IGDB.ClientID = "id"
	c.IGDB.ClientSecret = "secret"
	c.IGDB.IntervalMS = 0
	entries := []scanner.Cached{{Category: "games", Package: ps4.Package{TitleID: "CUSA12345", Title: "Bloodborne", Category: "gd"}}}
	out, _, e := s.Enrich(context.Background(), c, entries)
	if e != nil || len(out) != 0 || calls.Load() != 2 {
		t.Fatal(out, e, calls.Load())
	}
	s.Enrich(context.Background(), c, entries)
	if calls.Load() != 2 {
		t.Fatal("negative cache missed")
	}
}
func TestMapRich(t *testing.T) {
	g := Object{}
	json.Unmarshal([]byte(`{"id":7334,"summary":"Summary","involved_companies":[{"developer":true,"company":{"name":"Developer"}}],"release_dates":[{"platform":48,"date":1427068800}],"multiplayer_modes":[{"platform":48,"offlinemax":2,"onlinecoop":true,"onlinemax":5,"splitscreen":true}],"language_supports":[{"language":{"name":"English"},"language_support_type":{"name":"Audio"}}],"aggregated_rating":92,"screenshots":[{"image_id":"shot"}],"dlcs":[{"id":12,"name":"DLC"}]}`), &g)
	m := MapGame(g)
	if obj(obj(m["multiplayer"])["offline"])["max_players"] != float64(2) || m["schema_version"] != 1 || obj(obj(m["ratings"])["critics"])["score"] != float64(92) || len(arr(obj(m["relationships"])["dlcs"])) != 1 {
		t.Fatal(m)
	}
	if Similarity("Bloodborne™", "Bloodborne") != 1 || Similarity("Bloodborne", "Racing") >= 0.8 {
		t.Fatal("matching")
	}
}
