// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip1

package uname

import (
	"os"
	"runtime"
)

func getUtsname() (utsname, error) {
	nodename, _ := os.Hostname()
	if nodename == "" {
		nodename = "rusticated"
	}
	return utsname{
		Sysname:  "Rusticated",
		Nodename: nodename,
		Release:  "1.0.0",
		Version:  "wasm",
		Machine:  runtime.GOARCH,
	}, nil
}
