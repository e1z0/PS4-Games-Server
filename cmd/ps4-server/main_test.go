// SPDX-License-Identifier: GPL-2.0-only
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/scanner"
	"ps4server/internal/storage"
	"ps4server/internal/testpkg"
	"strings"
	"testing"
	"time"
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
	if e := run([]string{"serve", "-config", p}, strings.NewReader(""), &bytes.Buffer{}); e == nil || !strings.Contains(e.Error(), "unknown command") {
		t.Fatal("serve subcommand must be rejected", e)
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

// The helper executes the default server invocation, so locks, listeners and embedded
// assets are covered together on every native CI operating system.
func TestCLIServerProcess(t *testing.T) {
	if path := os.Getenv("PS4_CLI_TEST_CONFIG"); path != "" {
		if err := run([]string{"-config", path}, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	root := t.TempDir()
	pkgs := filepath.Join(root, "packages")
	if err := os.MkdirAll(pkgs, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgs, "game.pkg"), testpkg.Build(nil), 0600); err != nil {
		t.Fatal(err)
	}
	reserve := func() string {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		l.Close()
		return addr
	}
	c := config.Defaults()
	c.Listen = reserve()
	c.DownloadListen = reserve()
	for c.DownloadListen == c.Listen {
		c.DownloadListen = reserve()
	}
	c.PublicURL = "http://" + c.Listen
	c.DownloadURL = "http://" + c.DownloadListen
	c.DataDir = filepath.Join(root, "data")
	c.Configured = true
	c.Admin.Username = "admin"
	c.Admin.Hash, _ = config.Hash("integration password")
	c.Mappings = []config.Mapping{{ID: "main", Path: pkgs, Category: "games", Enabled: true}}
	path := filepath.Join(root, "config.json")
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(root, "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCLIServerProcess$")
	child.Env = append(os.Environ(), "PS4_CLI_TEST_CONFIG="+path)
	child.Stdout = log
	child.Stderr = log
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	done := false
	t.Cleanup(func() {
		if !done {
			child.Process.Kill()
			child.Wait()
		}
	})
	client := http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(30 * time.Second)
	for {
		res, e := client.Get(c.PublicURL + "/feeds/games")
		ready := false
		if e == nil {
			var cat scanner.Catalog
			json.NewDecoder(res.Body).Decode(&cat)
			res.Body.Close()
			ready = res.StatusCode == 200 && len(cat.Data) == 1
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			logs, _ := os.ReadFile(filepath.Join(root, "server.log"))
			t.Fatal("server never became ready", e, string(logs))
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, target := range []struct {
		url  string
		code int
	}{{c.PublicURL + "/library", 200}, {c.PublicURL + "/api/v1/settings", 401}, {c.DownloadURL + "/health", 200}, {c.DownloadURL + "/api/v1/library", 404}, {c.DownloadURL + "/feeds/games", 404}} {
		res, e := client.Get(target.url)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != target.code {
			t.Fatal(target.url, res.StatusCode)
		}
	}
	req, _ := http.NewRequest("GET", c.DownloadURL+"/pkg/main/game.pkg", nil)
	req.Header.Set("Range", "bytes=0-3")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 206 || !bytes.Equal(b, []byte{0x7f, 0x43, 0x4e, 0x54}) {
		t.Fatal("download listener range failed", res.StatusCode)
	}
	before, _ := os.ReadFile(path)
	if err = run([]string{"scan", "-config", path}, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "another process") {
		t.Fatal("concurrent CLI writer was accepted", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected writer modified configuration")
	}
	child.Process.Kill()
	child.Wait()
	done = true
	if err = run([]string{"scan", "-config", path}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal("process exit did not release writer locks", err)
	}
}

// SCM stop cancels this same parent context. Exercise real listener shutdown and
// lock release on every host, in addition to the Windows handler tests.
func TestServerParentCancellation(t *testing.T) {
	root := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	c := config.Defaults()
	c.Listen = addr
	c.PublicURL = "http://" + addr
	c.DataDir = filepath.Join(root, "data")
	c.Configured = true
	c.Scan.Startup = false
	c.Admin.Username = "admin"
	c.Admin.Hash, err = config.Hash("context test password")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.json")
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runContext(ctx, []string{"-config", path}, strings.NewReader(""), io.Discard, func() { close(ready) })
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatal("server exited before readiness", err)
	case <-time.After(30 * time.Second):
		t.Fatal("server startup timed out")
	}
	if err := run([]string{"health", "-config", path}, strings.NewReader(""), io.Discard); err != nil {
		t.Error(err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("server cancellation failed", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("server shutdown timed out")
	}
	// A second writer can acquire both locks only after graceful shutdown finishes.
	for _, lockPath := range []string{path + ".lock", filepath.Join(c.DataDir, ".writer.lock")} {
		lock, err := storage.Acquire(lockPath)
		if err != nil {
			t.Fatal("graceful shutdown retained writer lock", err)
		}
		if err := lock.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
