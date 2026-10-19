// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package sshd

import "os"

var (
	defaultShell     = getWindowsShell()
	defaultShellFlag = "/c"
)

func getWindowsShell() string {
	if c := os.Getenv("COMSPEC"); c != "" {
		return c
	}
	return "cmd.exe"
}
