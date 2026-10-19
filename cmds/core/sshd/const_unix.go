// Copyright 2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (!tinygo || tinygo.enable) && !plan9 && !windows && !wasip1

package sshd

var (
	defaultShell     = "/bin/sh"
	defaultShellFlag = "-c"
)
