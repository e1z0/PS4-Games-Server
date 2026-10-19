// SPDX-License-Identifier: GPL-2.0-only
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/metadata"
	"ps4server/internal/ps4"
	"ps4server/internal/scanner"
	"ps4server/internal/storage"
	"ps4server/internal/testpkg"
	"strings"
	"testing"
)

func request(t *testing.T, h http.Handler, method, path string, body any, cookie *http.Cookie, token string) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		json.NewEncoder(&b).Encode(body)
	}
	r := httptest.NewRequest(method, path, &b)
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("X-CSRF-Token", token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func authInfo(t *testing.T, w *httptest.ResponseRecorder) (*http.Cookie, string) {
	t.Helper()
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var b map[string]any
	json.Unmarshal(w.Body.Bytes(), &b)
	return w.Result().Cookies()[0], b["csrf"].(string)
}
func TestSetupAuthCSRFSettingsAndThrottle(t *testing.T) {
	dir := t.TempDir()
	c := config.Defaults()
	c.DataDir = filepath.Join(dir, "data")
	path := filepath.Join(dir, "config.json")
	config.Save(path, c)
	a, e := New(context.Background(), path, c)
	if e != nil {
		t.Fatal(e)
	}
	h := a.Handler(false)
	if request(t, h, "GET", "/api/v1/settings", nil, nil, "").Code != 401 {
		t.Fatal("settings public")
	}
	token := a.SetupToken()
	if request(t, h, "POST", "/api/v1/setup/open", map[string]string{"token": "wrong"}, nil, "").Code != 403 {
		t.Fatal("bad setup accepted")
	}
	cookie, csrf := authInfo(t, request(t, h, "POST", "/api/v1/setup/open", map[string]string{"token": token}, nil, ""))
	c.Admin.Username = "admin"
	c.Admin.Password = "very long test password"
	c.PublicURL = "http://nas:8080"
	if request(t, h, "PUT", "/api/v1/setup", c, cookie, "").Code != 403 {
		t.Fatal("CSRF missing")
	}
	w := request(t, h, "PUT", "/api/v1/setup", c, cookie, csrf)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if a.SetupToken() != "" || request(t, h, "POST", "/api/v1/setup/open", map[string]string{"token": token}, nil, "").Code != 403 {
		t.Fatal("token reused")
	}
	cookie, csrf = authInfo(t, request(t, h, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "very long test password"}, nil, ""))
	w = request(t, h, "GET", "/api/v1/settings", nil, cookie, "")
	if strings.Contains(w.Body.String(), "pbkdf2") || strings.Contains(w.Body.String(), "very long") {
		t.Fatal("secret exposed")
	}
	var edited config.Config
	json.Unmarshal(w.Body.Bytes(), &edited)
	edited.Listen = ":9090"
	edited.Scan.Startup = false
	w = request(t, h, "PUT", "/api/v1/settings", edited, cookie, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"restart_required":true`) {
		t.Fatal(w.Body.String())
	}
	saved, err := config.Load(path, false)
	if err != nil || saved.Scan.Startup || !config.Verify(saved.Admin.Hash, "very long test password") {
		t.Fatal(saved, err)
	}
	w = request(t, h, "GET", "/api/v1/directories?path="+dir, nil, cookie, "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	request(t, h, "POST", "/api/v1/auth/logout", map[string]any{}, cookie, csrf)
	if request(t, h, "GET", "/api/v1/settings", nil, cookie, "").Code != 401 {
		t.Fatal("logout failed")
	}
	for i := 0; i < 5; i++ {
		request(t, h, "POST", "/api/v1/auth/login", map[string]string{"username": "wrong", "password": "wrong"}, nil, "")
	}
	if request(t, h, "POST", "/api/v1/auth/login", map[string]string{"username": "wrong", "password": "wrong"}, nil, "").Code != 429 {
		t.Fatal("no throttle")
	}
	b, _ := os.ReadFile(path)
	if bytes.Contains(b, []byte("very long test password")) {
		t.Fatal("plaintext persisted")
	}
}
func TestFeedsRangesContainment(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "pkgs")
	os.MkdirAll(filepath.Join(root, "nested 猫"), 0700)
	pkg := filepath.Join(root, "nested 猫", "a #.pkg")
	data := testpkg.Build(nil)
	os.WriteFile(pkg, data, 0600)
	c := config.Defaults()
	c.DataDir = filepath.Join(dir, "data")
	c.PublicURL = "http://nas:8080"
	c.Mappings = []config.Mapping{{ID: "primary", Path: root, Category: "games", Enabled: true}}
	a, _ := New(context.Background(), filepath.Join(dir, "config.json"), c)
	if e := a.Engine().Run(context.Background(), c, false); e != nil {
		t.Fatal(e)
	}
	h := a.Handler(false)
	for _, path := range []string{"/games", "/games.json", "/feeds/games", "/library", "/"} {
		w := request(t, h, "GET", path, nil, nil, "")
		if w.Code != 200 {
			t.Fatal(path, w.Code)
		}
	}
	url := "/pkg/primary/" + scanner.Escape("nested 猫/a #.pkg")
	r := httptest.NewRequest("GET", url, nil)
	r.Header.Set("Range", "bytes=0-3")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[:4]) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(t, h, "HEAD", url, nil, nil, "")
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "16384" {
		t.Fatal(w.Code, w.Header())
	}
	for _, path := range []string{"/config.json", "/scan-cache.json", "/metadata-cache.json", "/pkg/primary/../config.json", "/covers/../scan-cache.json", "/api/v1/logs", "/pkg/primary/unknown.pkg"} {
		w := request(t, h, "GET", path, nil, nil, "")
		if w.Code == 200 {
			t.Fatal("internal exposed", path)
		}
	}
	outside := filepath.Join(dir, "secret.pkg")
	os.WriteFile(outside, []byte("secret"), 0600)
	os.Remove(pkg)
	if e := os.Symlink(outside, pkg); e == nil {
		if request(t, h, "GET", url, nil, nil, "").Code == 200 {
			t.Fatal("symlink escaped")
		}
	}
	if request(t, a.Handler(true), "GET", "/api/v1/library", nil, nil, "").Code != 404 {
		t.Fatal("download listener exposed API")
	}
	storage.JSON(filepath.Join(c.DataDir, "catalogs", "secret.json"), map[string]any{"password": "secret"})
	if request(t, h, "GET", "/secret", nil, nil, "").Code == 200 {
		t.Fatal("arbitrary json exposed")
	}
}
func TestOriginAndEmbeddedAssets(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	a, _ := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	for _, p := range []string{"/app.js", "/style.css", "/covers/fallback.svg", "/covers/fallback.png"} {
		w := request(t, a.Handler(false), "GET", p, nil, nil, "")
		if w.Code != 200 {
			t.Fatal(p, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "http://nas/api/v1/auth/login", strings.NewReader(`{}`))
	r.Header.Set("Origin", "http://attacker")
	w := httptest.NewRecorder()
	a.Handler(false).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("origin accepted")
	}
}

func TestOfflineRichMetadataRestoredWithoutScan(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.PublicURL = "http://nas:8080"
	c.Scan.Startup = false
	c.Mappings = []config.Mapping{{ID: "main", Path: t.TempDir(), Category: "games", Enabled: true}}
	entry := scanner.Cached{Mapping: "main", Root: c.Mappings[0].Path, Relative: "game.pkg", Category: "games", Package: ps4.Package{TitleID: "CUSA12345", Title: "Synthetic Game", Category: "gd"}}
	storage.JSON(filepath.Join(c.DataDir, "scan-cache.json"), scanner.State{Version: 1, Entries: []scanner.Cached{entry}})
	storage.JSON(filepath.Join(c.DataDir, "metadata-cache.json"), metadata.Cache{Version: 1, Titles: map[string]metadata.Entry{"CUSA12345": {Found: true, Data: metadata.Object{"schema_version": 1, "summary": "Offline summary"}}}})
	a, e := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	if e != nil {
		t.Fatal(e)
	}
	w := request(t, a.Handler(false), "GET", "/api/v1/library", nil, nil, "")
	if !strings.Contains(w.Body.String(), "Offline summary") || a.Engine().Status().Running {
		t.Fatal("cached rich details not restored", w.Body.String())
	}
}

func TestRasterFallbackForPS4(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	a, _ := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	w := request(t, a.Handler(false), "GET", "/covers/fallback.png", nil, nil, "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !ps4.ValidImage(w.Body.Bytes()) {
		t.Fatal("fallback is not a valid PS4-decodable raster image")
	}
	w = request(t, a.Handler(false), "HEAD", "/covers/fallback.png", nil, nil, "")
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") == "" {
		t.Fatal("fallback HEAD unsupported")
	}
}
