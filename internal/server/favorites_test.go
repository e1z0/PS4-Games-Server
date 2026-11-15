// SPDX-License-Identifier: GPL-2.0-only
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/scanner"
	"ps4server/internal/testpkg"
	"testing"
)

func TestFavoriteAuthenticationCSRFFeedAndRestart(t *testing.T) {
	c := config.Defaults()
	c.Configured = true
	c.PublicURL = "http://nas:8080"
	c.DataDir = t.TempDir()
	c.Admin.Username = "admin"
	c.Admin.Hash, _ = config.Hash("favorite test password")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "game.pkg"), testpkg.Build(nil), 0600); err != nil {
		t.Fatal(err)
	}
	c.Mappings = []config.Mapping{{ID: "main", Path: root, Category: "games", Enabled: true}}
	path := filepath.Join(t.TempDir(), "config.json")
	a, err := New(context.Background(), path, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	if err := a.Engine().Run(context.Background(), c, false); err != nil {
		t.Fatal(err)
	}
	h := a.Handler(false)
	u := scanner.PackageURL(c, a.Engine().Snapshot()[0])
	body := map[string]any{"package_url": u, "is_favorite": true}
	if w := request(t, h, "PUT", "/api/v1/favorites", body, nil, ""); w.Code != 401 {
		t.Fatal("unauthenticated write", w.Code)
	}
	setupResponse := httptest.NewRecorder()
	a.mu.Lock()
	setupSession := a.newSessionLocked(setupResponse, httptest.NewRequest("POST", "/", nil), true)
	a.mu.Unlock()
	if w := request(t, h, "PUT", "/api/v1/favorites", body, setupResponse.Result().Cookies()[0], setupSession.CSRF); w.Code != 403 {
		t.Fatal("setup session changed favorites", w.Code)
	}
	cookie, token := authInfo(t, request(t, h, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "favorite test password"}, nil, ""))
	if w := request(t, h, "PUT", "/api/v1/favorites", body, cookie, ""); w.Code != 403 {
		t.Fatal("missing CSRF", w.Code)
	}
	for _, invalid := range []map[string]any{{"package_url": u}, {"package_url": u, "is_favorite": nil}, {"package_url": u, "is_favorite": "true"}} {
		if w := request(t, h, "PUT", "/api/v1/favorites", invalid, cookie, token); w.Code != 400 {
			t.Fatal("bad payload accepted", w.Code)
		}
	}
	for _, url := range []string{"/pkg/main/game.pkg", "http://nas:8080/pkg/main/../config.json", "http://attacker/pkg/main/game.pkg"} {
		if w := request(t, h, "PUT", "/api/v1/favorites", map[string]any{"package_url": url, "is_favorite": true}, cookie, token); w.Code != 404 {
			t.Fatal("unknown package accepted", url, w.Code)
		}
	}
	if w := request(t, h, "PUT", "/api/v1/favorites", body, cookie, token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, route := range []string{"/feeds/games", "/games", "/games.json"} {
		w := request(t, h, http.MethodGet, route, nil, nil, "")
		var cat scanner.Catalog
		if err := json.Unmarshal(w.Body.Bytes(), &cat); err != nil || !cat.Data[u].Favorite {
			t.Fatal("public feed missing favorite", route, err, w.Body.String())
		}
	}
	if w := request(t, h, "GET", "/covers/../favorites.json", nil, nil, ""); w.Code == 200 {
		t.Fatal("internal preferences exposed")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = New(context.Background(), path, c)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Engine().Catalogs(c)["games"].Data[u].Favorite {
		t.Fatal("restart lost favorite")
	}
}
