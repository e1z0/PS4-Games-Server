// SPDX-License-Identifier: GPL-2.0-only
package config

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"ps4server/internal/storage"
	"regexp"
	"strconv"
	"strings"
)

type Mapping struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
}
type Admin struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	Hash     string `json:"password_hash,omitempty"`
}
type Scan struct {
	Startup         bool `json:"startup"`
	IntervalMinutes int  `json:"interval_minutes"`
}
type IGDB struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Enabled      bool   `json:"enabled"`
	PositiveDays int    `json:"positive_days"`
	NegativeDays int    `json:"negative_days"`
	IntervalMS   int    `json:"interval_ms"`
}
type Config struct {
	Version        int       `json:"version"`
	Configured     bool      `json:"configured"`
	Listen         string    `json:"listen"`
	DownloadListen string    `json:"download_listen"`
	PublicURL      string    `json:"public_url"`
	DownloadURL    string    `json:"download_url"`
	DataDir        string    `json:"data_dir"`
	Background     string    `json:"background"`
	Mappings       []Mapping `json:"mappings"`
	Scan           Scan      `json:"scan"`
	Admin          Admin     `json:"admin"`
	IGDB           IGDB      `json:"igdb"`
}

func Defaults() Config {
	return Config{Version: 1, Listen: ":8080", DataDir: "data", Mappings: []Mapping{}, Scan: Scan{Startup: true}, IGDB: IGDB{PositiveDays: 30, NegativeDays: 7, IntervalMS: 300}}
}

var SafeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)

// Environment fields seed absent JSON keys; presence, including false and zero, wins.
var Env = map[string]string{
	"PS4_LISTEN": "listen", "PS4_DOWNLOAD_LISTEN": "download_listen", "PS4_PUBLIC_URL": "public_url", "PS4_DOWNLOAD_URL": "download_url", "PS4_DATA_DIR": "data_dir", "PS4_BACKGROUND": "background", "PS4_MAPPINGS": "mappings", "PS4_SCAN_STARTUP": "scan.startup", "PS4_SCAN_INTERVAL_MINUTES": "scan.interval_minutes", "PS4_ADMIN_USERNAME": "admin.username", "PS4_ADMIN_PASSWORD": "admin.password", "PS4_IGDB_ENABLED": "igdb.enabled", "PS4_IGDB_CLIENT_ID": "igdb.client_id", "PS4_IGDB_CLIENT_SECRET": "igdb.client_secret", "PS4_IGDB_POSITIVE_DAYS": "igdb.positive_days", "PS4_IGDB_NEGATIVE_DAYS": "igdb.negative_days", "PS4_IGDB_INTERVAL_MS": "igdb.interval_ms",
}

func Load(path string, seed bool) (Config, error) {
	raw := map[string]any{}
	b, e := os.ReadFile(path)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return Config{}, e
	}
	if e == nil {
		if e = json.Unmarshal(b, &raw); e != nil {
			return Config{}, e
		}
	}
	if seed {
		for name, key := range Env {
			value, ok := os.LookupEnv(name)
			if !ok {
				continue
			}
			parts := strings.Split(key, ".")
			m := raw
			for _, p := range parts[:len(parts)-1] {
				v, exists := m[p]
				if !exists {
					v = map[string]any{}
					m[p] = v
				}
				var good bool
				m, good = v.(map[string]any)
				if !good {
					return Config{}, fmt.Errorf("invalid object %s", p)
				}
			}
			last := parts[len(parts)-1]
			var v any = value
			switch key {
			case "scan.startup", "igdb.enabled":
				x, err := strconv.ParseBool(value)
				if err != nil {
					return Config{}, fmt.Errorf("invalid %s", name)
				}
				v = x
			case "scan.interval_minutes", "igdb.positive_days", "igdb.negative_days", "igdb.interval_ms":
				x, err := strconv.Atoi(value)
				if err != nil || x < 0 {
					return Config{}, fmt.Errorf("invalid %s", name)
				}
				v = x
			case "mappings":
				if err := json.Unmarshal([]byte(value), &v); err != nil {
					return Config{}, fmt.Errorf("invalid %s", name)
				}
			}
			if _, exists := m[last]; !exists {
				if key == "admin.password" && m["password_hash"] != nil {
					continue
				}
				m[last] = v
			}
		}
	}
	c := Defaults()
	b, _ = json.Marshal(raw)
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	if c.Admin.Password != "" {
		c.Admin.Hash, e = Hash(c.Admin.Password)
		if e != nil {
			return c, e
		}
		c.Admin.Password = ""
	}
	if c.Admin.Hash != "" && c.Admin.Username != "" && c.PublicURL != "" {
		c.Configured = true
	}
	if e = c.Validate(); e != nil {
		return c, e
	}
	if seed {
		e = Save(path, c)
	}
	return c, e
}
func Save(path string, c Config) error {
	if c.Admin.Password != "" {
		var e error
		c.Admin.Hash, e = Hash(c.Admin.Password)
		if e != nil {
			return e
		}
		c.Admin.Password = ""
	}
	return storage.JSON(path, c)
}
func (c Config) Validate() error {
	if c.Version != 1 {
		return errors.New("unsupported configuration version")
	}
	for _, s := range []string{c.Listen, c.DownloadListen} {
		if s != "" {
			_, p, e := net.SplitHostPort(s)
			n, err := strconv.Atoi(p)
			if e != nil || err != nil || n < 1 || n > 65535 {
				return errors.New("listener must have host:port with port 1–65535")
			}
		}
	}
	if c.Listen == "" {
		return errors.New("listen is required")
	}
	for _, s := range []string{c.PublicURL, c.DownloadURL} {
		if s == "" {
			continue
		}
		u, e := url.Parse(s)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
			return errors.New("public URLs must be HTTP(S) origins")
		}
		h := u.Hostname()
		if h == "0.0.0.0" || h == "::" || h == "*" {
			return errors.New("public URL must be PS4-reachable, not a wildcard")
		}
	}
	if c.DataDir == "" || c.Scan.IntervalMinutes < 0 || c.IGDB.IntervalMS < 0 || c.IGDB.PositiveDays < 0 || c.IGDB.NegativeDays < 0 {
		return errors.New("invalid data directory or negative interval")
	}
	seen := map[string]bool{}
	for _, m := range c.Mappings {
		if !SafeName.MatchString(m.ID) || !SafeName.MatchString(m.Category) || seen[m.ID] || !filepath.IsAbs(m.Path) {
			return errors.New("mappings require unique safe IDs, safe categories and absolute paths")
		}
		seen[m.ID] = true
	}
	if c.Configured && (c.Admin.Username == "" || !ValidHash(c.Admin.Hash) || c.PublicURL == "") {
		return errors.New("configured server requires credentials and public URL")
	}
	return nil
}
func Token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func Hash(p string) (string, error) {
	if len(p) < 12 || len(p) > 1024 {
		return "", errors.New("password must contain 12–1024 bytes")
	}
	salt := Token()
	k, e := pbkdf2.Key(sha256.New, p, []byte(salt), 210000, 32)
	return "pbkdf2-sha256$210000$" + salt + "$" + base64.RawStdEncoding.EncodeToString(k), e
}
func ValidHash(h string) bool {
	p := strings.Split(h, "$")
	if len(p) != 4 || p[0] != "pbkdf2-sha256" || p[1] != "210000" || len(p[2]) != 43 {
		return false
	}
	b, e := base64.RawStdEncoding.DecodeString(p[3])
	return e == nil && len(b) == 32
}
func Verify(h, p string) bool {
	if !ValidHash(h) || len(p) > 1024 {
		return false
	}
	v := strings.Split(h, "$")
	k, e := pbkdf2.Key(sha256.New, p, []byte(v[2]), 210000, 32)
	return e == nil && subtle.ConstantTimeCompare([]byte(base64.RawStdEncoding.EncodeToString(k)), []byte(v[3])) == 1
}
func (c Config) Redacted() Config {
	c.Admin.Hash = ""
	c.Admin.Password = ""
	c.IGDB.ClientSecret = ""
	return c
}
func (c Config) Downloads() string {
	if c.DownloadURL != "" {
		return strings.TrimRight(c.DownloadURL, "/")
	}
	return strings.TrimRight(c.PublicURL, "/")
}
func (c Config) CoversDir() string { return filepath.Join(c.DataDir, "covers") }
