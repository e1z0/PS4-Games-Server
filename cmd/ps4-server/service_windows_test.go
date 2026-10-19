// SPDX-License-Identifier: GPL-2.0-only
package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestWindowsServiceLifecycle(t *testing.T) {
	for name, stop := range map[string]svc.Cmd{"stop": svc.Stop, "shutdown": svc.Shutdown} {
		t.Run(name, func(t *testing.T) {
			requests := make(chan svc.ChangeRequest, 4)
			changes := make(chan svc.Status, 16)
			done := make(chan uint32, 1)
			started := make(chan struct{})
			allowReady := make(chan struct{})
			h := &windowsService{out: io.Discard, run: func(ctx context.Context, ready func()) error {
				close(started)
				select {
				case <-allowReady:
				case <-ctx.Done():
					return ctx.Err()
				}
				ready()
				<-ctx.Done()
				return nil
			}}
			t.Cleanup(func() { requests <- svc.ChangeRequest{Cmd: svc.Stop} })
			go func() { _, code := h.Execute(nil, requests, changes); done <- code }()
			expect := func(state svc.State) {
				t.Helper()
				select {
				case status := <-changes:
					if status.State != state {
						t.Fatalf("state %d, want %d", status.State, state)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("service status timed out")
				}
			}
			expect(svc.StartPending)
			<-started
			requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
			expect(svc.StartPending)
			close(allowReady)
			expect(svc.Running)
			requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
			expect(svc.Running)
			requests <- svc.ChangeRequest{Cmd: stop}
			expect(svc.StopPending)
			select {
			case code := <-done:
				if code != 0 {
					t.Fatalf("clean stop returned %d", code)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("service did not stop")
			}
		})
	}
}

func TestWindowsServiceStartupFailure(t *testing.T) {
	want := errors.New("listener already bound")
	h := &windowsService{out: io.Discard, run: func(context.Context, func()) error { return want }}
	requests := make(chan svc.ChangeRequest)
	changes := make(chan svc.Status, 16)
	specific, code := h.Execute(nil, requests, changes)
	if !specific || code == 0 || !errors.Is(h.err, want) {
		t.Fatalf("startup error lost: %t %d %v", specific, code, h.err)
	}
	for len(changes) > 0 {
		if status := <-changes; status.State == svc.Running {
			t.Fatal("failed startup reported running")
		}
	}
}
