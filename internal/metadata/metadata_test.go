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
			if !strings.Contains(string(b), "platforms = (48)") {
				t.Error("IGDB query lacks PS4 filter")
			}
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
	progress := [][2]int{}
	out, warn, e := s.EnrichWithProgress(context.Background(), c, entries, func(done, total int, current string) {
		progress = append(progress, [2]int{done, total})
	})
	if len(progress) != 2 || progress[0] != [2]int{0, 1} || progress[1] != [2]int{1, 1} {
		t.Fatal("title-sharing progress is incorrect", progress)
	}
	if e != nil || len(warn) > 0 || out["CUSA12345"]["summary"] != "Rich summary" || tokens.Load() != 2 || images.Load() != 1 {
		t.Fatal(out, warn, e, tokens.Load(), queries.Load())
	}
	q := queries.Load()
	// A fresh service must reuse both metadata and image validation after restart.
	restarted := New()
	restarted.TokenURL, restarted.GamesURL, restarted.ImageBase = s.TokenURL, s.GamesURL, s.ImageBase
	cachedProgress := [][2]int{}
	restarted.EnrichWithProgress(context.Background(), c, entries, func(done, total int, current string) {
		cachedProgress = append(cachedProgress, [2]int{done, total})
	})
	if len(cachedProgress) != 1 || cachedProgress[0] != [2]int{0, 0} {
		t.Fatal("cached titles appeared as fetch work", cachedProgress)
	}
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

func TestInvalidResponseAndUpdateOnlyDoNotNegativeCache(t *testing.T) {
	var calls atomic.Int32
	var response atomic.Value
	response.Store("null")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			json.NewEncoder(w).Encode(Object{"access_token": "token", "expires_in": 3600})
			return
		}
		calls.Add(1)
		w.Write([]byte(response.Load().(string)))
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
	entries := []scanner.Cached{{Category: "updates", Package: ps4.Package{TitleID: "CUSA12345", Title: "Patch", Category: "gp"}}}
	s.Enrich(context.Background(), c, entries)
	if calls.Load() != 0 {
		t.Fatal("update queried")
	}
	entries[0].Package.Category = "gd"
	for _, body := range []string{"null", "[] {}", "[] trailing"} {
		response.Store(body)
		_, warnings, e := s.Enrich(context.Background(), c, entries)
		if e != nil || len(warnings) == 0 {
			t.Fatal("malformed response accepted", body, e, warnings)
		}
	}
	cache := Cache{}
	storage.Read(filepath.Join(c.DataDir, "metadata-cache.json"), &cache)
	if len(cache.Titles) != 0 {
		t.Fatal("invalid response cached negatively")
	}
}

func TestConservativeSequelAndRepeatedWordMatching(t *testing.T) {
	for _, titles := range [][2]string{{"Call of Duty Modern Warfare 2", "Call of Duty Modern Warfare 3"}, {"Final Fantasy VII", "Final Fantasy VIII"}, {"Man", "Man Man Man Man"}} {
		score := Similarity(titles[0], titles[1])
		if score < 0 || score > 1 {
			t.Fatal("similarity outside 0–1", score)
		}
		if score >= 0.8 {
			t.Fatal("different sequel accepted", titles, score)
		}
	}
	if Similarity("Bloodborne™ Game of the Year Edition", "Bloodborne") != 1 {
		t.Fatal("base game edition was not recognized")
	}
}

func TestNegativeCacheExpiryAndFirstRequestOutage(t *testing.T) {
	var calls atomic.Int32
	var outage atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			json.NewEncoder(w).Encode(Object{"access_token": "token", "expires_in": 3600})
			return
		}
		calls.Add(1)
		if outage.Load() {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode([]Object{})
	}))
	defer ts.Close()
	s := New()
	s.TokenURL = ts.URL + "/token"
	s.GamesURL = ts.URL + "/games"
	now := time.Now()
	s.Now = func() time.Time { return now }
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.IGDB.Enabled = true
	c.IGDB.ClientID = "id"
	c.IGDB.ClientSecret = "secret"
	c.IGDB.IntervalMS = 0
	entries := []scanner.Cached{{Category: "games", Package: ps4.Package{TitleID: "CUSA12345", Title: "Synthetic Game", Category: "gd"}}}
	s.Enrich(context.Background(), c, entries)
	s.Enrich(context.Background(), c, entries)
	if calls.Load() != 1 {
		t.Fatal("negative cache missed")
	}
	now = now.Add(8 * 24 * time.Hour)
	s.Enrich(context.Background(), c, entries)
	if calls.Load() != 2 {
		t.Fatal("negative cache did not expire")
	}
	outage.Store(true)
	entries[0].Package.TitleID = "CUSA54321"
	_, warnings, err := s.Enrich(context.Background(), c, entries)
	if err != nil || len(warnings) == 0 {
		t.Fatal(err, warnings)
	}
	cache := Cache{}
	storage.Read(filepath.Join(c.DataDir, "metadata-cache.json"), &cache)
	if _, present := cache.Titles["CUSA54321"]; present {
		t.Fatal("fresh network failure became negative cache")
	}
}

func TestStartupRestoreDefersImageValidationAndNetwork(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.PublicURL = "http://nas:8080"
	c.IGDB.Enabled = true
	c.IGDB.ClientID = "must-not-contact-igdb"
	c.IGDB.ClientSecret = "must-not-contact-igdb"
	cache := Cache{Version: 1, Titles: map[string]Entry{"CUSA12345": {Found: true, Expires: time.Now().Add(-time.Hour), Data: Object{"summary": "Cached details", "media": Object{"screenshots": []any{Object{"image_id": "savedshot"}}}}}}}
	if err := storage.JSON(filepath.Join(c.DataDir, "metadata-cache.json"), cache); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(c.CoversDir(), "igdb", "screenshots", "CUSA12345", "savedshot.jpg")
	if err := os.MkdirAll(filepath.Dir(image), 0700); err != nil {
		t.Fatal(err)
	}
	// Existing non-image bytes prove Restore performs only a lightweight stat.
	// They remain rejected by actual serving and by the next validating scan.
	if err := os.WriteFile(image, []byte("needs validation during scan"), 0600); err != nil {
		t.Fatal(err)
	}
	service := New()
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer ts.Close()
	service.TokenURL = ts.URL
	service.GamesURL = ts.URL
	service.ImageBase = ts.URL + "/"
	entries := []scanner.Cached{{Category: "games", Package: ps4.Package{TitleID: "CUSA12345", Title: "Synthetic", Category: "gd"}}}
	out, _, err := service.Restore(context.Background(), c, entries)
	if err != nil || calls != 0 || out["CUSA12345"]["summary"] != "Cached details" {
		t.Fatal("startup performed network work or lost cache", calls, err, out)
	}
	shot := obj(arr(obj(out["CUSA12345"]["media"])["screenshots"])[0])
	if shot["cached"] != true || shot["url"] != "http://nas:8080/covers/igdb/screenshots/CUSA12345/savedshot.jpg" {
		t.Fatal("cached URL not restored", shot)
	}
	c.IGDB.Enabled = false
	out, _, err = service.Enrich(context.Background(), c, entries)
	if err != nil {
		t.Fatal(err)
	}
	shot = obj(arr(obj(out["CUSA12345"]["media"])["screenshots"])[0])
	if shot["cached"] != false || calls != 0 {
		t.Fatal("normal scan skipped validation or performed offline network work", shot, calls)
	}
}

func TestCancelledBatchKeepsCompletedLookups(t *testing.T) {
	var queries atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			json.NewEncoder(w).Encode(Object{"access_token": "token", "expires_in": 3600})
			return
		}
		queries.Add(1)
		json.NewEncoder(w).Encode([]Object{{"id": 123, "name": "Synthetic Game", "summary": "Saved before cancellation"}})
	}))
	defer ts.Close()
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.IGDB.Enabled, c.IGDB.ClientID, c.IGDB.ClientSecret = true, "id", "secret"
	c.IGDB.IntervalMS = 0
	service := func() *Service {
		s := New()
		s.TokenURL, s.GamesURL = ts.URL+"/token", ts.URL+"/games"
		return s
	}
	entries := []scanner.Cached{
		{Category: "games", Package: ps4.Package{TitleID: "CUSA12345", Title: "Synthetic Game", Category: "gd"}},
		{Category: "games", Package: ps4.Package{TitleID: "CUSA12346", Title: "Synthetic Game", Category: "gd"}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, err := service().EnrichWithProgress(ctx, c, entries, func(done, total int, current string) {
		if done == 1 {
			cancel()
		}
	})
	if err != context.Canceled {
		t.Fatal("expected cancellation", err)
	}
	cache := Cache{}
	if err := storage.Read(filepath.Join(c.DataDir, "metadata-cache.json"), &cache); err != nil || !cache.Titles["CUSA12345"].Found || len(cache.Titles) != 1 {
		t.Fatal("completed lookup lost after cancellation", cache, err)
	}
	before := queries.Load()
	out, warnings, err := service().Enrich(context.Background(), c, entries[:1])
	if err != nil || len(warnings) != 0 || queries.Load() != before || out["CUSA12345"]["summary"] != "Saved before cancellation" {
		t.Fatal("restart fetched completed title again", out, warnings, err, queries.Load(), before)
	}
}
