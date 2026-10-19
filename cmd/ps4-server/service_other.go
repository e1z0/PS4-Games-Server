// SPDX-License-Identifier: GPL-2.0-only
//go:build !windows

package main

import (
	"errors"
	"io"
)

func platformMain() (bool, error) { return false, nil }
func serviceCommand(args []string, out io.Writer) error {
	return errors.New("Windows service commands are available only on Windows")
}
