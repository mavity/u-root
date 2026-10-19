// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package tty

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func findTTY(stdin io.Reader) (string, error) {
	f, ok := stdin.(*os.File)
	if !ok {
		f = os.Stdin
	}
	var mode uint32
	err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode)
	if err == nil {
		return "CONIN$", nil
	}
	// Also check stdout
	err = windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &mode)
	if err == nil {
		return "CONOUT$", nil
	}
	return "", errors.New("not a tty")
}
