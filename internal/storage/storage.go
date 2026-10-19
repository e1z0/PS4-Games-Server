// SPDX-License-Identifier: GPL-2.0-only
package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Write replaces a file atomically and preserves its timestamp when unchanged.
func Write(path string, data []byte, mode fs.FileMode) error {
	old, err := Bytes(path)
	if err == nil && bytes.Equal(old, data) {
		return os.Chmod(path, mode)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceFile(f.Name(), path)
}
func JSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return Write(path, append(b, '\n'), 0600)
}
func Read(path string, v any) error {
	b, e := Bytes(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func Within(root, path string) bool {
	r, e := filepath.Rel(root, path)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

// Resolve rejects traversal and symlinks that leave an explicitly allowed root.
func Resolve(root, rel string) (string, error) {
	if filepath.IsAbs(rel) || strings.Contains(rel, "\\") {
		return "", fs.ErrPermission
	}
	base, e := filepath.EvalSymlinks(root)
	if e != nil {
		return "", e
	}
	p, e := filepath.EvalSymlinks(filepath.Join(base, filepath.FromSlash(rel)))
	if e != nil {
		return "", e
	}
	if !Within(base, p) {
		return "", fs.ErrPermission
	}
	return p, nil
}

// Bytes opens relative to a directory handle; Windows handles permit atomic replacement.
func Bytes(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
