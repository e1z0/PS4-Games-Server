// SPDX-License-Identifier: GPL-2.0-only
package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/metadata"
	"ps4server/internal/ps4"
	"ps4server/internal/scanner"
	"ps4server/internal/storage"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed ui/*
var assets embed.FS

type session struct {
	CSRF    string
	Expires time.Time
	Setup   bool
}
type attempt struct {
	Count int
	Since time.Time
}
type App struct {
	mu       sync.Mutex
	cfg      config.Config
	initial  config.Config
	path     string
	engine   *scanner.Engine
	token    string
	sessions map[string]session
	attempts map[string]attempt
	ctx      context.Context
	stop     context.CancelFunc
	dataLock *storage.Lock
}

func New(ctx context.Context, path string, c config.Config) (*App, error) {
	dataLock, err := storage.Acquire(filepath.Join(c.DataDir, ".writer.lock"))
	if err != nil {
		return nil, err
	}
	e, err := scanner.New(c)
	if err != nil {
		dataLock.Close()
		return nil, err
	}
	ctx, stop := context.WithCancel(ctx)
	e.Enricher = metadata.New()
	e.RestoreMetadata(ctx, c)
	a := &App{cfg: c, initial: c, path: path, engine: e, ctx: ctx, stop: stop, dataLock: dataLock, sessions: map[string]session{}, attempts: map[string]attempt{}}
	if !c.Configured {
		a.token = config.Token()
	}
	return a, nil
}

// Close cancels jobs before releasing the data writer lock. Stop HTTP requests first.
func (a *App) Close() error {
	a.stop()
	engine := a.Engine()
	engine.Cancel()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for engine.Status().Running {
		select {
		case <-deadline.C:
			return fmt.Errorf("scan did not stop; data lock retained")
		case <-time.After(time.Millisecond):
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dataLock.Close()
}
func (a *App) SetupToken() string      { a.mu.Lock(); defer a.mu.Unlock(); return a.token }
func (a *App) Engine() *scanner.Engine { a.mu.Lock(); defer a.mu.Unlock(); return a.engine }
func (a *App) Config() config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.cfg
	c.DataDir = a.initial.DataDir
	c.Listen = a.initial.Listen
	c.DownloadListen = a.initial.DownloadListen
	return c
}
func (a *App) Schedule() {
	go func() { tick := time.NewTicker(time.Second); defer tick.Stop(); a.schedule(tick.C, time.Now()) }()
}
func (a *App) schedule(ticks <-chan time.Time, started time.Time) {
	last := started
	if c := a.Config(); c.Configured && c.Scan.Startup {
		a.Engine().Start(a.ctx, c, false)
	}
	for {
		select {
		case <-a.ctx.Done():
			a.Engine().Cancel()
			return
		case now, ok := <-ticks:
			if !ok {
				return
			}
			c := a.Config()
			if c.Configured && c.Scan.IntervalMinutes > 0 && now.Sub(last) >= time.Duration(c.Scan.IntervalMinutes)*time.Minute {
				if a.Engine().Start(a.ctx, c, false) == nil {
					last = now
				}
			}
		}
	}
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, s string) {
	jsonOut(w, status, map[string]string{"error": s})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		fail(w, 415, "application/json required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		fail(w, 400, "invalid request JSON")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		fail(w, 400, "unexpected trailing JSON")
		return false
	}
	return true
}
func (a *App) Handler(downloadOnly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: http: data:; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Cache-Control", "no-store")
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || u.Host != r.Host {
					fail(w, 403, "cross-origin request rejected")
					return
				}
			}
		}
		p := r.URL.Path
		if p == "/health" {
			jsonOut(w, 200, map[string]string{"status": "ok"})
			return
		}
		if strings.HasPrefix(p, "/pkg/") || strings.HasPrefix(p, "/covers/") || p == "/background" {
			if r.Method != "GET" && r.Method != "HEAD" {
				fail(w, 405, "GET or HEAD required")
				return
			}
			a.download(w, r)
			return
		}
		if downloadOnly {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(p, "/api/v1/") {
			w.Header().Set("Cache-Control", "no-store")
			a.api(w, r)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			fail(w, 405, "GET or HEAD required")
			return
		}
		if p == "/" || p == "/library" || p == "/setup" || p == "/login" || p == "/admin" {
			c := a.Config()
			if p == "/admin" {
				s, ok := a.auth(r)
				if !ok || s.Setup {
					http.Redirect(w, r, "/login", 303)
					return
				}
			}
			if p == "/setup" && c.Configured {
				http.NotFound(w, r)
				return
			}
			a.asset(w, r, "index.html")
			return
		}
		if p == "/app.js" || p == "/style.css" || p == "/fallback.svg" {
			a.asset(w, r, p[1:])
			return
		}
		category := strings.TrimSuffix(strings.TrimPrefix(p, "/"), ".json")
		if strings.HasPrefix(category, "feeds/") {
			category = strings.TrimPrefix(category, "feeds/")
		}
		if config.SafeName.MatchString(category) {
			if cat, ok := a.Engine().Catalogs(a.Config())[category]; ok {
				jsonOut(w, 200, cat)
				return
			}
		}
		http.NotFound(w, r)
	})
}
func (a *App) asset(w http.ResponseWriter, r *http.Request, name string) {
	b, e := assets.ReadFile("ui/" + name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	switch filepath.Ext(name) {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}
func (a *App) auth(r *http.Request) (session, bool) {
	cookie, e := r.Cookie("ps4_session")
	if e != nil {
		return session{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[cookie.Value]
	if !ok || time.Now().After(s.Expires) {
		delete(a.sessions, cookie.Value)
		return session{}, false
	}
	return s, true
}
func (a *App) newSessionLocked(w http.ResponseWriter, r *http.Request, setup bool) session {
	for k, v := range a.sessions {
		if time.Now().After(v.Expires) {
			delete(a.sessions, k)
		}
	}
	if len(a.sessions) > 1000 {
		a.sessions = map[string]session{}
	}
	if old, e := r.Cookie("ps4_session"); e == nil {
		delete(a.sessions, old.Value)
	}
	id := config.Token()
	s := session{CSRF: config.Token(), Expires: time.Now().Add(12 * time.Hour), Setup: setup}
	a.sessions[id] = s
	http.SetCookie(w, &http.Cookie{Name: "ps4_session", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil || strings.HasPrefix(a.cfg.PublicURL, "https://"), MaxAge: 43200})
	return s
}
func csrf(r *http.Request, s session) bool {
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRF)) == 1
}
func (a *App) api(w http.ResponseWriter, r *http.Request) {
	route := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	if route == "feeds" && r.Method == "GET" {
		urls := map[string]string{}
		c := a.Config()
		for category := range a.Engine().Catalogs(c) {
			urls[category] = strings.TrimRight(c.PublicURL, "/") + "/feeds/" + category
		}
		jsonOut(w, 200, urls)
		return
	}
	if route == "library/status" && r.Method == "GET" {
		j := a.Engine().Status()
		// Public library progress deliberately omits server paths and private errors.
		jsonOut(w, 200, map[string]any{"running": j.Running, "phase": j.Phase,
			"visited": j.Visited, "total": j.Total, "metadata_done": j.MetadataDone,
			"metadata_total": j.MetadataTotal, "packages": len(a.Engine().Snapshot()),
			"warnings": len(j.Errors), "cancelled": j.Cancelled, "finished": j.Finished})
		return
	}
	if route == "library" && r.Method == "GET" {
		jsonOut(w, 200, a.Engine().Catalogs(a.Config()))
		return
	}
	if route == "setup/open" && r.Method == "POST" {
		var b struct {
			Token string `json:"token"`
		}
		if !decode(w, r, &b) {
			return
		}
		a.mu.Lock()
		valid := a.token != "" && !a.cfg.Configured && subtle.ConstantTimeCompare([]byte(b.Token), []byte(a.token)) == 1
		c := a.cfg.Redacted()
		secretSet := a.cfg.IGDB.ClientSecret != ""
		if !valid {
			a.mu.Unlock()
			fail(w, 403, "invalid or expired setup token")
			return
		}
		s := a.newSessionLocked(w, r, true)
		a.mu.Unlock()
		jsonOut(w, 200, map[string]any{"csrf": s.CSRF, "config": c, "igdb_secret_set": secretSet})
		return
	}
	if route == "auth/login" && r.Method == "POST" {
		a.login(w, r)
		return
	}
	s, ok := a.auth(r)
	if !ok {
		fail(w, 401, "login required")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" && !csrf(r, s) {
		fail(w, 403, "CSRF token required")
		return
	}
	if route == "auth/session" && r.Method == "GET" {
		c := a.Config()
		jsonOut(w, 200, map[string]any{"csrf": s.CSRF, "setup": s.Setup, "config": c.Redacted(), "igdb_secret_set": c.IGDB.ClientSecret != ""})
		return
	}
	if route == "setup" && r.Method == "PUT" && s.Setup {
		a.settings(w, r, true)
		return
	}
	if route == "directories" && r.Method == "GET" {
		a.directories(w, r)
		return
	}
	if s.Setup {
		fail(w, 403, "complete setup first")
		return
	}
	switch route {
	case "auth/logout":
		if r.Method == "POST" {
			a.mu.Lock()
			if c, e := r.Cookie("ps4_session"); e == nil {
				delete(a.sessions, c.Value)
			}
			a.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "ps4_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
			jsonOut(w, 200, map[string]bool{"ok": true})
			return
		}
	case "settings":
		if r.Method == "GET" {
			a.mu.Lock()
			c := a.cfg.Redacted()
			a.mu.Unlock()
			jsonOut(w, 200, c)
			return
		}
		if r.Method == "PUT" {
			a.settings(w, r, false)
			return
		}
	case "status":
		if r.Method == "GET" {
			a.mu.Lock()
			restart := a.cfg.Listen != a.initial.Listen || a.cfg.DownloadListen != a.initial.DownloadListen || a.cfg.DataDir != a.initial.DataDir
			a.mu.Unlock()
			jsonOut(w, 200, map[string]any{"scan": a.Engine().Status(), "packages": len(a.Engine().Snapshot()), "restart_required": restart})
			return
		}
	case "logs":
		if r.Method == "GET" {
			jsonOut(w, 200, a.Engine().Logs())
			return
		}
	case "scan":
		if r.Method == "POST" {
			var b struct {
				Force bool `json:"force"`
			}
			if !decode(w, r, &b) {
				return
			}
			if e := a.Engine().Start(a.ctx, a.Config(), b.Force); e != nil {
				fail(w, 409, e.Error())
				return
			}
			jsonOut(w, 202, map[string]bool{"started": true})
			return
		}
	case "scan/cancel":
		if r.Method == "POST" {
			a.Engine().Cancel()
			jsonOut(w, 200, map[string]bool{"cancelled": true})
			return
		}
	case "metadata/overrides":
		a.overrides(w, r)
		return
	}
	fail(w, 404, "unknown endpoint or method")
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &b) {
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.mu.Lock()
	for k, v := range a.attempts {
		if time.Since(v.Since) > 15*time.Minute {
			delete(a.attempts, k)
		}
	}
	x := a.attempts[ip]
	if x.Since.IsZero() {
		x.Since = time.Now()
	}
	if x.Count >= 5 || len(a.attempts) >= 10000 {
		a.mu.Unlock()
		w.Header().Set("Retry-After", "900")
		fail(w, 429, "too many login attempts; try again in 15 minutes")
		return
	}
	x.Count++
	a.attempts[ip] = x
	c := a.cfg
	a.mu.Unlock()
	if !c.Configured || b.Username != c.Admin.Username || !config.Verify(c.Admin.Hash, b.Password) {
		fail(w, 401, "invalid credentials")
		return
	}
	a.mu.Lock()
	if a.cfg.Admin.Username != c.Admin.Username || a.cfg.Admin.Hash != c.Admin.Hash || !a.cfg.Configured {
		a.mu.Unlock()
		fail(w, 401, "credentials changed; sign in again")
		return
	}
	delete(a.attempts, ip)
	s := a.newSessionLocked(w, r, false)
	a.mu.Unlock()
	jsonOut(w, 200, map[string]string{"csrf": s.CSRF})
}
func Writable(dir string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".write-test-*")
	if e != nil {
		return e
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}
func (a *App) settings(w http.ResponseWriter, r *http.Request, setup bool) {
	var c config.Config
	if !decode(w, r, &c) {
		return
	}
	if c.Mappings == nil {
		c.Mappings = []config.Mapping{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if setup && (a.cfg.Configured || a.token == "") {
		fail(w, 409, "setup already completed")
		return
	}
	old := a.cfg
	if c.Admin.Password != "" {
		h, e := config.Hash(c.Admin.Password)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		c.Admin.Hash = h
		c.Admin.Password = ""
	} else {
		c.Admin.Hash = old.Admin.Hash
	}
	if c.IGDB.ClientSecret == "" {
		c.IGDB.ClientSecret = old.IGDB.ClientSecret
	}
	c.Configured = true
	if e := c.Validate(); e != nil {
		fail(w, 400, e.Error())
		return
	}
	for _, m := range c.Mappings {
		if m.Enabled {
			if e := CheckPath(m.Path); e != nil { // Existing unavailable mappings can be retained while editing unrelated settings.
				found := false
				for _, prev := range old.Mappings {
					if prev.ID == m.ID && prev.Path == m.Path {
						found = true
					}
				}
				if !found {
					fail(w, 400, "Cannot read folder "+m.Path+" ("+m.ID+"). Choose an existing readable server folder; Docker paths must be mounted in Compose.")
					return
				}
			}
		}
	}
	if e := Writable(c.DataDir); e != nil {
		fail(w, 400, "Cannot write to data folder "+c.DataDir+". Choose writable storage and check the container UID/GID permissions.")
		return
	}
	var engine *scanner.Engine
	var replacementLock *storage.Lock
	if setup && !a.dataLock.Matches(filepath.Join(c.DataDir, ".writer.lock")) {
		var err error
		replacementLock, err = storage.Acquire(filepath.Join(c.DataDir, ".writer.lock"))
		if err != nil {
			fail(w, 409, "data directory is in use by another process")
			return
		}
		defer func() {
			if replacementLock != nil {
				replacementLock.Close()
			}
		}()
	}
	if setup {
		var e error
		engine, e = scanner.New(c)
		if e != nil {
			fail(w, 400, "cannot open data directory")
			return
		}
		engine.Enricher = metadata.New()
		engine.RestoreMetadata(a.ctx, c)
	}
	if e := config.Save(a.path, c); e != nil {
		fail(w, 500, "could not persist settings")
		return
	}
	a.cfg = c
	if setup {
		// Setup can swap data engines, but the HTTP listeners are already bound.
		a.initial.DataDir = c.DataDir
		if replacementLock != nil {
			a.dataLock.Close()
			a.dataLock = replacementLock
			replacementLock = nil
		}
		a.engine = engine
		a.token = ""
	}
	changedCredentials := setup || c.Admin.Username != old.Admin.Username || c.Admin.Hash != old.Admin.Hash
	if changedCredentials {
		a.sessions = map[string]session{}
	}
	restart := c.Listen != a.initial.Listen || c.DownloadListen != a.initial.DownloadListen || c.DataDir != a.initial.DataDir
	jsonOut(w, 200, map[string]any{"ok": true, "restart_required": restart, "login_required": changedCredentials})
}
func (a *App) directories(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		p, _ = os.Getwd()
	}
	if !filepath.IsAbs(p) {
		fail(w, 400, "absolute server path required")
		return
	}
	entries, e := os.ReadDir(p)
	if e != nil {
		fail(w, 400, "Cannot open "+p+". Check that the folder exists and the server can read it. In Docker, select a mounted container folder such as /packages.")
		return
	}
	dirs := []string{}
	for _, d := range entries {
		if d.IsDir() {
			dirs = append(dirs, filepath.Join(p, d.Name()))
		}
	}
	sort.Strings(dirs)
	roots := []string{string(filepath.Separator)}
	if runtime.GOOS == "windows" {
		roots = []string{}
		for drive := 'A'; drive <= 'Z'; drive++ {
			root := string(drive) + ":\\"
			if info, err := os.Stat(root); err == nil && info.IsDir() {
				roots = append(roots, root)
			}
		}
	}
	jsonOut(w, 200, map[string]any{"path": p, "parent": filepath.Dir(p), "directories": dirs, "roots": roots, "path_help": "Paths belong to the server. In Docker use container paths; add host folders through Compose bind mounts."})
}
func (a *App) overrides(w http.ResponseWriter, r *http.Request) {
	dir := a.Config().DataDir
	if r.Method == "GET" {
		m, e := metadata.LoadOverrides(dir)
		if e != nil {
			fail(w, 500, "invalid overrides file")
			return
		}
		jsonOut(w, 200, m)
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "GET or PUT required")
		return
	}
	var m map[string]map[string]any
	if !decode(w, r, &m) {
		return
	}
	for id := range m {
		if !config.SafeName.MatchString(id) {
			fail(w, 400, "invalid title ID")
			return
		}
	}
	if e := storage.JSON(filepath.Join(dir, "metadata-overrides.json"), m); e != nil {
		fail(w, 500, "override write failed")
		return
	}
	jsonOut(w, 200, map[string]string{"status": "saved; run a scan to apply"})
}
func serveFile(w http.ResponseWriter, r *http.Request, root, rel string) {
	root, e := filepath.EvalSymlinks(root)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	canonical, e := storage.Resolve(root, rel)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	base, e := os.OpenRoot(root)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	defer base.Close()
	// Open the resolved relative path so contained absolute symlinks work too.
	// OpenRoot still prevents any subsequent symlink swap from leaving the root.
	resolvedRelative, e := filepath.Rel(root, canonical)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	f, e := base.Open(resolvedRelative)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil || !s.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, s.Name(), s.ModTime(), f)
}
func (a *App) download(w http.ResponseWriter, r *http.Request) {
	c := a.Config()
	p := r.URL.Path
	if p == "/background" {
		if c.Background == "" || !ps4.ValidImageFile(c.Background) {
			http.NotFound(w, r)
			return
		}
		serveFile(w, r, filepath.Dir(c.Background), filepath.Base(c.Background))
		return
	}
	if strings.HasPrefix(p, "/pkg/") {
		rel := strings.TrimPrefix(p, "/pkg/")
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		for _, v := range a.Engine().Snapshot() {
			if v.Mapping != parts[0] || v.Relative != parts[1] {
				continue
			}
			// The published snapshot authorizes this root. Pending mapping edits
			// must not break downloads belonging to the current/completing job.
			serveFile(w, r, v.Root, v.Relative)
			return
		}
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(p, "/covers/")
	if rel == "fallback.svg" || rel == "fallback.png" {
		a.asset(w, r, rel)
		return
	}
	if ext := strings.ToLower(filepath.Ext(rel)); ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		http.NotFound(w, r)
		return
	}
	resolved, e := storage.Resolve(c.CoversDir(), rel)
	if e != nil || !ps4.ValidImageFile(resolved) {
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, c.CoversDir(), rel)
}

// CheckPath is used by CLI and setup to verify server-native directory access.
func CheckPath(path string) error {
	s, e := os.Stat(path)
	if e != nil {
		return e
	}
	if !s.IsDir() {
		return fs.ErrInvalid
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.ReadDir(1)
	if e == io.EOF {
		return nil
	}
	return e
}
