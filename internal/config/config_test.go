// SPDX-License-Identifier: GPL-2.0-only
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSeedPresenceAndPassword(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"scan":{"startup":false,"interval_minutes":0}}`), 0600)
	t.Setenv("PS4_SCAN_STARTUP", "true")
	t.Setenv("PS4_SCAN_INTERVAL_MINUTES", "60")
	t.Setenv("PS4_ADMIN_USERNAME", "admin")
	t.Setenv("PS4_ADMIN_PASSWORD", "long secret password")
	t.Setenv("PS4_PUBLIC_URL", "http://nas:8080")
	c, e := Load(p, true)
	if e != nil {
		t.Fatal(e)
	}
	if c.Scan.Startup || c.Scan.IntervalMinutes != 0 || !c.Configured || !Verify(c.Admin.Hash, "long secret password") {
		t.Fatal(c)
	}
	c.PublicURL = "http://edited:8080"
	Save(p, c)
	c, e = Load(p, true)
	if e != nil || c.PublicURL != "http://edited:8080" {
		t.Fatal(e, c)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "long secret") {
		t.Fatal("plaintext persisted")
	}
	s, _ := os.Stat(p)
	if runtime.GOOS != "windows" && s.Mode().Perm() != 0600 {
		t.Fatal(s.Mode())
	}
}
func TestValidation(t *testing.T) {
	c := Defaults()
	c.PublicURL = "http://0.0.0.0:8080"
	if c.Validate() == nil {
		t.Fatal("wildcard accepted")
	}
	p := filepath.Join(t.TempDir(), "c.json")
	t.Setenv("PS4_SCAN_STARTUP", "banana")
	if _, e := Load(p, true); e == nil {
		t.Fatal("bad env accepted")
	}
}

func TestInvalidSeedStillRejectedWhenPresent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := Save(p, Defaults()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PS4_PUBLIC_URL", "http://0.0.0.0:8080")
	if _, err := Load(p, true); err == nil {
		t.Fatal("explicit invalid environment value ignored")
	}
}
