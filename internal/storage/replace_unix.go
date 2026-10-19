// SPDX-License-Identifier: GPL-2.0-only
//go:build !windows

package storage

import "os"

func replaceFile(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
