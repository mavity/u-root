// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build plan9

package uname

import (
	"os"
	"runtime"
)

func getUtsname() (utsname, error) {
	nodename, _ := os.Hostname()
	if nodename == "" {
		nodename = "plan9"
	}
	return utsname{
		Sysname:  "Plan9",
		Nodename: nodename,
		Release:  "4",
		Version:  "Plan 9 from Bell Labs",
		Machine:  runtime.GOARCH,
	}, nil
}
