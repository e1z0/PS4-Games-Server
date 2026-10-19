// SPDX-License-Identifier: GPL-2.0-only
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"ps4server/internal/config"
	"ps4server/internal/metadata"
	"ps4server/internal/scanner"
	"ps4server/internal/server"
	"ps4server/internal/storage"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var version = "0.1.0-dev"

func main() {
	if e := entry(); e != nil {
		fmt.Fprintln(os.Stderr, "Error:", e)
		os.Exit(1)
	}
}
func entry() error {
	if handled, err := platformMain(); handled || err != nil {
		return err
	}
	return run(os.Args[1:], os.Stdin, os.Stdout)
}
func run(args []string, in io.Reader, out io.Writer) error {
	return runContext(context.Background(), args, in, out, nil)
}
func runContext(parent context.Context, args []string, in io.Reader, out io.Writer, ready func()) error {
	if len(args) > 0 && args[0] == "service" {
		return serviceCommand(args[1:], out)
	}
	command := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}
	f := flag.NewFlagSet("ps4-server", flag.ContinueOnError)
	f.SetOutput(out)
	path := f.String("config", "config.json", "configuration JSON path")
	force := f.Bool("force", false, "force package metadata inspection (scan)")
	healthURL := f.String("url", "", "health-check server URL")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	if command == "version" {
		fmt.Fprintf(out, "ps4-server %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return nil
	}
	if command == "help" {
		fmt.Fprintln(out, "ps4-server [-config config.json] starts the server.\nps4-server <scan|init|validate|password-reset|health|version|help> [-config config.json]\nscan: -force; health: -url http://127.0.0.1:8080\nWindows: service install [-config config.json]; service <start|stop|status|uninstall>\npassword-reset reads a new password from standard input (12–1024 bytes). See docs/configuration.md.")
		return nil
	}
	if command == "health" {
		u := *healthURL
		if u == "" {
			c, e := config.Load(*path, false)
			if e != nil {
				return e
			}
			host, port, _ := net.SplitHostPort(c.Listen)
			if host == "" || host == "0.0.0.0" || host == "::" {
				host = "127.0.0.1"
			}
			u = "http://" + net.JoinHostPort(host, port)
		}
		client := http.Client{Timeout: 5 * time.Second}
		r, e := client.Get(strings.TrimRight(u, "/") + "/health")
		if e != nil {
			return errors.New("health check failed to connect")
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("health HTTP %d", r.StatusCode)
		}
		fmt.Fprintln(out, "healthy")
		return nil
	}
	switch command {
	case "", "scan", "init", "validate", "password-reset":
	default:
		return fmt.Errorf("unknown command %q; run ps4-server help", command)
	}
	if command != "validate" {
		lock, err := storage.Acquire(*path + ".lock")
		if err != nil {
			return err
		}
		defer lock.Close()
	}
	if command == "init" {
		if _, e := os.Stat(*path); e == nil {
			return errors.New("configuration already exists")
		}
	}
	c, e := config.Load(*path, command != "validate" && command != "password-reset")
	if e != nil {
		return e
	}
	abs, e := filepath.Abs(*path)
	if e != nil {
		return e
	}
	switch command {
	case "init":
		fmt.Fprintln(out, "Created", abs, "— run ps4-server and open the setup URL.")
		return nil
	case "validate":
		if _, e := os.Stat(*path); e != nil {
			return e
		}
		fmt.Fprintf(out, "Configuration valid (setup complete: %t)\n", c.Configured)
		return nil
	case "password-reset":
		if c.Admin.Username == "" {
			return errors.New("administrator username is not configured")
		}
		b, e := io.ReadAll(io.LimitReader(in, 1027))
		if e != nil {
			return e
		}
		c.Admin.Hash, e = config.Hash(strings.TrimRight(string(b), "\r\n"))
		if e != nil {
			return e
		}
		if e = config.Save(*path, c); e != nil {
			return e
		}
		fmt.Fprintln(out, "Password reset. Restart any running server to apply the new password and invalidate sessions.")
		return nil
	case "scan":
		if !c.Configured {
			return errors.New("complete setup or seed administrator credentials and a public URL first")
		}
		dataLock, err := storage.Acquire(filepath.Join(c.DataDir, ".writer.lock"))
		if err != nil {
			return err
		}
		defer dataLock.Close()
		engine, e := scanner.New(c)
		if e != nil {
			return e
		}
		engine.Enricher = metadata.New()
		ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
		defer stop()
		e = engine.Run(ctx, c, *force)
		json.NewEncoder(out).Encode(engine.Status())
		return e
	}
	if e = server.Writable(c.DataDir); e != nil {
		return errors.New("data directory is not writable")
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	app, e := server.New(ctx, abs, c)
	if e != nil {
		return e
	}
	defer app.Close()
	listener, e := net.Listen("tcp", c.Listen)
	if e != nil {
		return e
	}
	defer listener.Close()
	servers := []*http.Server{{Handler: app.Handler(false), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}}
	listeners := []net.Listener{listener}
	if c.DownloadListen != "" {
		l, e := net.Listen("tcp", c.DownloadListen)
		if e != nil {
			return e
		}
		defer l.Close()
		listeners = append(listeners, l)
		servers = append(servers, &http.Server{Handler: app.Handler(true), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second})
	}
	fmt.Fprintf(out, "PS4 Games Server %s listening on %s\n", version, listener.Addr())
	if token := app.SetupToken(); token != "" {
		host, port, _ := net.SplitHostPort(c.Listen)
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "localhost"
		}
		fmt.Fprintf(out, "Setup: http://%s/setup#token=%s\nReplace localhost with this server's address when connecting remotely.\n", net.JoinHostPort(host, port), token)
	}
	if ready != nil {
		ready()
	}
	app.Schedule()
	errs := make(chan error, len(servers))
	for i, srv := range servers {
		go func(srv *http.Server, l net.Listener) { errs <- srv.Serve(l) }(srv, listeners[i])
	}
	select {
	case <-ctx.Done():
	case e = <-errs:
		if errors.Is(e, http.ErrServerClosed) {
			e = nil
		}
	}
	stop()
	app.Engine().Cancel()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		srv.Shutdown(shutdown)
	}
	for app.Engine().Status().Running {
		select {
		case <-shutdown.Done():
			return errors.New("scan shutdown timed out")
		case <-time.After(20 * time.Millisecond):
		}
	}
	return e
}
