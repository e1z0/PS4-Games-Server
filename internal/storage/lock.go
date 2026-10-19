// SPDX-License-Identifier: GPL-2.0-only
package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// Lock is an OS-owned exclusive lock. The empty sidecar is never unlinked, so
// concurrent processes cannot lock different inodes. Crashes release the lock.
type Lock struct {
	file *os.File
	path string
}

func lockPath(path string) (string, error) {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(path)), nil
}
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	canonical, err := lockPath(path)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(canonical, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockExclusive(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("another process is using %s: %w", canonical, err)
	}
	return &Lock{file: f, path: canonical}, nil
}
func (l *Lock) Matches(path string) bool { p, err := lockPath(path); return err == nil && p == l.path }
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unlockExclusive(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if err == nil {
		err = closeErr
	}
	return err
}
