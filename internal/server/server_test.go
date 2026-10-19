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
	"runtime"
	"strings"
	"testing"
	"time"
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
	if a != nil {
		t.Cleanup(func() { a.Close() })
	}
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
	if a != nil {
		t.Cleanup(func() { a.Close() })
	}
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
	if a != nil {
		t.Cleanup(func() { a.Close() })
	}
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
	if a != nil {
		t.Cleanup(func() { a.Close() })
	}
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
	if a != nil {
		t.Cleanup(func() { a.Close() })
	}
	w := request(t, a.Handler(false), "GET", "/covers/fallback.png", nil, nil, "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !ps4.ValidImage(w.Body.Bytes()) {
		t.Fatal("fallback is not a valid PS4-decodable raster image")
	}
	w = request(t, a.Handler(false), "HEAD", "/covers/fallback.png", nil, nil, "")
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") == "" {
		t.Fatal("fallback HEAD unsupported")
	}
}

func TestSetupListenerChangesRequireRestart(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	a, err := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error("close application:", err)
		}
	})
	h := a.Handler(false)
	cookie, token := authInfo(t, request(t, h, "POST", "/api/v1/setup/open", map[string]string{"token": a.SetupToken()}, nil, ""))
	c.Listen = ":8081"
	c.DownloadListen = ":8082"
	c.Admin.Username = "admin"
	c.Admin.Password = "setup test password"
	c.PublicURL = "http://nas:8081"
	w := request(t, h, "PUT", "/api/v1/setup", c, cookie, token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"restart_required":true`) {
		t.Fatal("setup falsely applied listeners", w.Code, w.Body.String())
	}
	if a.Config().Listen != ":8080" || a.Config().DownloadListen != "" {
		t.Fatal("runtime listeners changed without binding")
	}
}
func TestCanonicalFeedLinksForReservedCategories(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.PublicURL = "http://nas:8080"
	c.Mappings = []config.Mapping{{ID: "main", Path: t.TempDir(), Category: "admin", Enabled: true}}
	a, err := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error("close application:", err)
		}
	})
	w := request(t, a.Handler(false), "GET", "/api/v1/feeds", nil, nil, "")
	if !strings.Contains(w.Body.String(), "http://nas:8080/feeds/admin") {
		t.Fatal(w.Body.String())
	}
	w = request(t, a.Handler(false), "GET", "/feeds/admin", nil, nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"DATA"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestContainedAbsoluteSymlinkDownload(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "real.pkg")
	os.WriteFile(pkg, testpkg.Build(nil), 0600)
	if err := os.Symlink(pkg, filepath.Join(root, "alias.pkg")); err != nil {
		t.Skip(err)
	}
	w := httptest.NewRecorder()
	serveFile(w, httptest.NewRequest("GET", "/pkg/main/alias.pkg", nil), root, "alias.pkg")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), testpkg.Build(nil)) {
		t.Fatal("contained symlink failed", w.Code)
	}
}
func TestDirectoryBrowserStartsAtNativePath(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	a, err := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error("close application:", err)
		}
	})
	h := a.Handler(false)
	cookie, _ := authInfo(t, request(t, h, "POST", "/api/v1/setup/open", map[string]string{"token": a.SetupToken()}, nil, ""))
	w := request(t, h, "GET", "/api/v1/directories", nil, cookie, "")
	var body struct {
		Path  string   `json:"path"`
		Roots []string `json:"roots"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 200 || !filepath.IsAbs(body.Path) || len(body.Roots) == 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authenticated data is cacheable")
	}
}

func TestExclusiveDataWriterAndSetupLockTransfer(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	path := filepath.Join(t.TempDir(), "c.json")
	a, err := New(context.Background(), path, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	if other, err := New(context.Background(), path, c); err == nil {
		other.Close()
		t.Fatal("second server acquired active data directory")
	}
	h := a.Handler(false)
	cookie, token := authInfo(t, request(t, h, "POST", "/api/v1/setup/open", map[string]string{"token": a.SetupToken()}, nil, ""))
	oldDir := c.DataDir
	newDir := t.TempDir()
	foreign, err := storage.Acquire(filepath.Join(newDir, ".writer.lock"))
	if err != nil {
		t.Fatal(err)
	}
	c.DataDir = newDir
	c.Admin.Username = "admin"
	c.Admin.Password = "setup lock password"
	c.PublicURL = "http://nas:8080"
	w := request(t, h, "PUT", "/api/v1/setup", c, cookie, token)
	if w.Code != 409 {
		t.Fatal("setup accepted an active data directory", w.Code, w.Body.String())
	}
	foreign.Close()
	w = request(t, h, "PUT", "/api/v1/setup", c, cookie, token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if lock, err := storage.Acquire(filepath.Join(newDir, ".writer.lock")); err == nil {
		lock.Close()
		t.Fatal("new data directory not locked")
	}
	lock, err := storage.Acquire(filepath.Join(oldDir, ".writer.lock"))
	if err != nil {
		t.Fatal("old data lock not released", err)
	}
	lock.Close()
	a.Close()
	lock, err = storage.Acquire(filepath.Join(newDir, ".writer.lock"))
	if err != nil {
		t.Fatal("closed server kept lock", err)
	}
	lock.Close()
}

func TestStartupAndScheduledScansCaptureCurrentMappings(t *testing.T) {
	c := config.Defaults()
	c.Configured = true
	c.DataDir = t.TempDir()
	c.PublicURL = "http://nas:8080"
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "game.pkg"), testpkg.Build(nil), 0600)
	c.Mappings = []config.Mapping{{ID: "main", Path: root, Category: "games", Enabled: true}}
	c.Scan.IntervalMinutes = 1
	a, err := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	ticks := make(chan time.Time)
	done := make(chan struct{})
	started := time.Now()
	go func() { a.schedule(ticks, started); close(done) }()
	waitUntil := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for !check() {
			if time.Now().After(deadline) {
				t.Fatal("scheduled scan did not finish", a.Engine().Status())
			}
			time.Sleep(time.Millisecond)
		}
	}
	waitUntil(func() bool { return !a.Engine().Status().Running && len(a.Engine().Snapshot()) == 1 })
	a.mu.Lock()
	a.cfg.Mappings = []config.Mapping{{ID: "main", Path: root, Category: "games", Enabled: false}}
	a.mu.Unlock()
	ticks <- started.Add(time.Minute)
	waitUntil(func() bool { return !a.Engine().Status().Running && len(a.Engine().Snapshot()) == 0 })
	a.stop()
	<-done
}

func TestRangeBeyondFourGiB(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "large.pkg")
	os.WriteFile(pkg, testpkg.Build(nil), 0600)
	f, err := os.OpenFile(pkg, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	const offset = int64(1<<32) + 7
	if err = f.Truncate(offset + 4); err != nil {
		f.Close()
		t.Skip(err)
	}
	_, err = f.WriteAt([]byte("TAIL"), offset)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.PublicURL = "http://nas:8080"
	c.Mappings = []config.Mapping{{ID: "main", Path: root, Category: "games", Enabled: true}}
	a, err := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	if err = a.Engine().Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/pkg/main/large.pkg", nil)
	r.Header.Set("Range", "bytes=4294967303-4294967306")
	w := httptest.NewRecorder()
	a.Handler(false).ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "TAIL" || w.Header().Get("Content-Range") != "bytes 4294967303-4294967306/4294967307" {
		t.Fatal("64-bit range failed", w.Code, w.Header(), w.Body.String())
	}
}

func TestCustomBackgroundAndImageIsolation(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.Background = filepath.Join(t.TempDir(), "custom.png")
	png, err := assets.ReadFile("ui/fallback.png")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(c.Background, png, 0600)
	a, err := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	w := request(t, a.Handler(false), "GET", "/background", nil, nil, "")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), png) {
		t.Fatal("custom background not served", w.Code)
	}
	w = request(t, a.Handler(false), "HEAD", "/background", nil, nil, "")
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("background HEAD failed")
	}
	os.WriteFile(c.Background, []byte(`{"password":"private"}`), 0600)
	w = request(t, a.Handler(false), "GET", "/background", nil, nil, "")
	if w.Code != 404 || strings.Contains(w.Body.String(), "private") {
		t.Fatal("invalid background exposed arbitrary data")
	}
}

func TestNewUnreadableMappingRejected(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix permission enforcement")
	}
	c := config.Defaults()
	c.DataDir = t.TempDir()
	a, err := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	cookie, token := authInfo(t, request(t, a.Handler(false), "POST", "/api/v1/setup/open", map[string]string{"token": a.SetupToken()}, nil, ""))
	dir := t.TempDir()
	os.Chmod(dir, 0000)
	defer os.Chmod(dir, 0700)
	c.Admin.Username = "admin"
	c.Admin.Password = "unreadable test password"
	c.PublicURL = "http://nas:8080"
	c.Mappings = []config.Mapping{{ID: "main", Path: dir, Category: "games", Enabled: true}}
	w := request(t, a.Handler(false), "PUT", "/api/v1/setup", c, cookie, token)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "mapping directory unavailable") {
		t.Fatal("unreadable mapping accepted", w.Code, w.Body.String())
	}
}

type gatedMetadata struct {
	entered chan struct{}
	release chan struct{}
}

func (g gatedMetadata) Enrich(ctx context.Context, _ config.Config, _ []scanner.Cached) (map[string]map[string]any, []string, error) {
	close(g.entered)
	select {
	case <-g.release:
		return nil, nil, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}
func TestPendingMappingEditPreservesPublishedDownloads(t *testing.T) {
	c := config.Defaults()
	c.Configured = true
	c.DataDir = t.TempDir()
	c.PublicURL = "http://nas:8080"
	c.Admin.Username = "admin"
	c.Admin.Hash, _ = config.Hash("mapping test password")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "game.pkg"), testpkg.Build(nil), 0600)
	c.Mappings = []config.Mapping{{ID: "main", Path: root, Category: "games", Enabled: true}}
	a, err := New(context.Background(), filepath.Join(t.TempDir(), "c.json"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	if err = a.Engine().Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	gate := gatedMetadata{make(chan struct{}), make(chan struct{})}
	a.Engine().Enricher = gate
	if err = a.Engine().Start(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	<-gate.entered
	h := a.Handler(false)
	cookie, token := authInfo(t, request(t, h, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "mapping test password"}, nil, ""))
	edited := c
	edited.Mappings = []config.Mapping{{ID: "main", Path: root, Category: "games", Enabled: false}}
	w := request(t, h, "PUT", "/api/v1/settings", edited, cookie, token)
	if w.Code != 200 {
		close(gate.release)
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(t, h, "HEAD", "/pkg/main/game.pkg", nil, nil, "")
	close(gate.release)
	if w.Code != 200 {
		t.Fatal("pending mapping edit broke a published download", w.Code)
	}
	deadline := time.Now().Add(30 * time.Second)
	for a.Engine().Status().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if a.Engine().Status().Running {
		t.Fatal("scan did not finish")
	}
	a.Engine().Enricher = nil
	if err = a.Engine().Run(context.Background(), a.Config(), false); err != nil {
		t.Fatal(err)
	}
	if w = request(t, h, "GET", "/pkg/main/game.pkg", nil, nil, ""); w.Code != 404 {
		t.Fatal("completed mapping removal still served package", w.Code)
	}
}
