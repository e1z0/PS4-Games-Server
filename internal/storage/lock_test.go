// SPDX-License-Identifier: GPL-2.0-only
package storage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProcessLock(t *testing.T) {
	path := os.Getenv("PS4_TEST_LOCK_PATH")
	if path != "" {
		lock, err := Acquire(path)
		if os.Getenv("PS4_TEST_LOCK_ACTION") == "crash" && err == nil {
			os.Exit(0)
		}
		if err == nil {
			lock.Close()
			os.Exit(2)
		}
		os.Exit(0)
	}
	path = filepath.Join(t.TempDir(), "writer.lock")
	lock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err = Acquire(path); err == nil {
		t.Fatal("second writer acquired held lock")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestProcessLock$")
	child.Env = append(os.Environ(), "PS4_TEST_LOCK_PATH="+path)
	if b, err := child.CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	lock.Close()
	child = exec.Command(os.Args[0], "-test.run=^TestProcessLock$")
	child.Env = append(os.Environ(), "PS4_TEST_LOCK_PATH="+path, "PS4_TEST_LOCK_ACTION=crash")
	if b, err := child.CombinedOutput(); err != nil {
		t.Fatal("crash-lock child failed", err, string(b))
	}
	again, err := Acquire(path)
	if err != nil {
		t.Fatal("closed/crashed process lock not released", err)
	}
	again.Close()
}
