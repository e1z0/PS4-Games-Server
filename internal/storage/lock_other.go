// SPDX-License-Identifier: GPL-2.0-only
//go:build !linux && !darwin && !windows

package storage

import (
	"errors"
	"os"
)

func lockExclusive(*os.File) error {
	return errors.New("process locks require Linux, macOS or Windows")
}
func unlockExclusive(*os.File) error { return nil }
