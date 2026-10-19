// SPDX-License-Identifier: GPL-2.0-only
package main

import (
	"bytes"
	"path/filepath"
	"ps4server/internal/config"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	for _, cmd := range []string{"init", "validate", "version"} {
		var out bytes.Buffer
		if e := run([]string{cmd, "-config", p}, strings.NewReader(""), &out); e != nil || out.Len() == 0 {
			t.Fatal(cmd, e)
		}
	}
	if e := run([]string{"init", "-config", p}, strings.NewReader(""), &bytes.Buffer{}); e == nil {
		t.Fatal("init overwrote existing")
	}
	c, e := config.Load(p, false)
	if e != nil {
		t.Fatal(e)
	}
	c.Admin.Username = "admin"
	config.Save(p, c)
	if e := run([]string{"password-reset", "-config", p}, strings.NewReader("new long password\n"), &bytes.Buffer{}); e != nil {
		t.Fatal(e)
	}
	c, _ = config.Load(p, false)
	if !config.Verify(c.Admin.Hash, "new long password") {
		t.Fatal("password not reset")
	}
}
