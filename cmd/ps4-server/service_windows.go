// SPDX-License-Identifier: GPL-2.0-only
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"ps4server/internal/config"
	"ps4server/internal/storage"
)

const serviceName = "PS4GamesServer"

// Interactive double-click launches use the executable folder, regardless of
// Explorer's working directory. Explicit CLI arguments retain normal CLI paths.
func platformMain() (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, err
	}
	if !isService {
		if len(os.Args) == 1 {
			exe, err := os.Executable()
			if err != nil {
				return false, err
			}
			return false, os.Chdir(filepath.Dir(exe))
		}
		return false, nil
	}
	f := flag.NewFlagSet("Windows service", flag.ContinueOnError)
	path := f.String("config", "", "absolute configuration path")
	dir := f.String("working-directory", "", "absolute working directory")
	if err := f.Parse(os.Args[1:]); err != nil {
		return true, err
	}
	if f.NArg() != 0 || !filepath.IsAbs(*path) || !filepath.IsAbs(*dir) {
		return true, errors.New("invalid service paths; reinstall the service")
	}
	if err := os.Chdir(*dir); err != nil {
		return true, err
	}
	log, err := os.OpenFile(filepath.Join(filepath.Dir(*path), "service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return true, err
	}
	defer log.Close()
	h := &windowsService{path: *path, out: log}
	err = svc.Run(serviceName, h)
	if err == nil {
		err = h.err
	}
	if err != nil {
		fmt.Fprintln(log, time.Now().Format(time.RFC3339), "Service error:", err)
	}
	return true, err
}

type windowsService struct {
	path string
	out  io.Writer
	// run is injectable for lifecycle tests; production uses the real HTTP server.
	run func(context.Context, func()) error
	err error
}

func (h *windowsService) Execute(args []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	start := h.run
	if start == nil {
		start = func(ctx context.Context, ready func()) error {
			return runContext(ctx, []string{"-config", h.path}, os.Stdin, h.out, ready)
		}
	}
	current := svc.Status{State: svc.StartPending, WaitHint: 30000, CheckPoint: 1}
	changes <- current
	go func() { done <- start(ctx, func() { ready <- struct{}{} }) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ready:
			if current.State == svc.StartPending {
				current = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
				changes <- current
			}
		case err := <-done:
			h.err = err
			if err != nil {
				return true, 1
			}
			return false, 0
		case request, ok := <-requests:
			if !ok {
				cancel()
				requests = nil
				continue
			}
			switch request.Cmd {
			case svc.Interrogate:
				changes <- current
			case svc.Stop, svc.Shutdown:
				current = svc.Status{State: svc.StopPending, WaitHint: 30000, CheckPoint: 1}
				changes <- current
				cancel()
			}
		case <-ticker.C:
			if current.State == svc.StartPending || current.State == svc.StopPending {
				current.CheckPoint++
				changes <- current
			}
		}
	}
}

func serviceCommand(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("use service install, start, stop, status or uninstall")
	}
	action := args[0]
	switch action {
	case "install", "start", "stop", "status", "uninstall":
	default:
		return fmt.Errorf("unknown service action %q", action)
	}
	f := flag.NewFlagSet("service "+action, flag.ContinueOnError)
	f.SetOutput(out)
	path := f.String("config", "config.json", "configuration used by service install")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected service arguments")
	}
	if action != "install" && f.NFlag() != 0 {
		return errors.New("-config applies only to service install; other actions use the installed service")
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("open Windows services (use PowerShell as administrator): %w", err)
	}
	defer m.Disconnect()
	if action == "install" {
		abs, err := filepath.Abs(*path)
		if err != nil {
			return err
		}
		lock, err := storage.Acquire(abs + ".lock")
		if err != nil {
			return fmt.Errorf("stop the console server before installing: %w", err)
		}
		defer lock.Close()
		c, err := config.Load(abs, false)
		if err != nil {
			return err
		}
		if !c.Configured {
			return errors.New("run the EXE and complete browser setup before installing the service")
		}
		dataLock, err := storage.Acquire(filepath.Join(c.DataDir, ".writer.lock"))
		if err != nil {
			return err
		}
		defer dataLock.Close()
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		s, err := m.CreateService(serviceName, exe, mgr.Config{
			DisplayName: "PS4 Games Server", Description: "PS4 package library, feeds and downloads",
			StartType: mgr.StartAutomatic, DelayedAutoStart: true,
			ServiceStartName: `NT AUTHORITY\LocalService`,
		}, "-config", abs, "-working-directory", dir)
		if err != nil {
			return fmt.Errorf("install service (administrator rights required; an existing service must be uninstalled first): %w", err)
		}
		defer s.Close()
		fmt.Fprintln(out, "Installed PS4GamesServer (automatic delayed start, Local Service account).")
		fmt.Fprintln(out, "Grant Local Service write access to configuration/data and read access to the EXE/packages; see WINDOWS.md. Then run: ps4-server.exe service start")
		return nil
	}
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("open PS4GamesServer: %w", err)
	}
	defer s.Close()
	switch action {
	case "status":
		status, err := s.Query()
		if err != nil {
			return err
		}
		fmt.Fprintln(out, serviceState(status.State))
	case "start":
		if err := s.Start(); err != nil {
			return err
		}
		if err := waitService(s, svc.Running); err != nil {
			return err
		}
		fmt.Fprintln(out, "Service running.")
	case "stop":
		if err := stopService(s); err != nil {
			return err
		}
		fmt.Fprintln(out, "Service stopped.")
	case "uninstall":
		if err := stopService(s); err != nil {
			return err
		}
		if err := s.Delete(); err != nil {
			return err
		}
		fmt.Fprintln(out, "Service uninstalled; configuration, data and packages retained.")
	}
	return nil
}
func serviceState(state svc.State) string {
	switch state {
	case svc.Running:
		return "running"
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "starting"
	case svc.StopPending:
		return "stopping"
	default:
		return fmt.Sprintf("state %d", state)
	}
}
func stopService(s *mgr.Service) error {
	status, err := s.Query()
	if err != nil {
		return err
	}
	if status.State == svc.Stopped {
		return nil
	}
	if status.State != svc.StopPending {
		if _, err := s.Control(svc.Stop); err != nil {
			return err
		}
	}
	return waitService(s, svc.Stopped)
}
func waitService(s *mgr.Service, want svc.State) error {
	deadline := time.Now().Add(45 * time.Second)
	for {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State == want {
			return nil
		}
		if want == svc.Running && status.State == svc.Stopped {
			return fmt.Errorf("service stopped during startup (Windows exit %d, service exit %d); check service.log and folder permissions", status.Win32ExitCode, status.ServiceSpecificExitCode)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service is still %s; check service.log and Services", serviceState(status.State))
		}
		time.Sleep(200 * time.Millisecond)
	}
}
