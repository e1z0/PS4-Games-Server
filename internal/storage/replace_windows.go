// SPDX-License-Identifier: GPL-2.0-only
package storage

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// Windows readers without delete sharing can transiently block atomic replacement.
// Retry replacement without deleting the previous file or exposing a partial write.
func replaceFile(oldpath, newpath string) error {
	var err error
	for i := 0; i < 8; i++ {
		err = os.Rename(oldpath, newpath)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.Errno(5)) && !errors.Is(err, syscall.Errno(32)) && !errors.Is(err, syscall.Errno(33)) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return err
}
