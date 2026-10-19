// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package uname

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

func getUtsname() (utsname, error) {
	nodename, _ := os.Hostname()
	if nodename == "" {
		nodename = "localhost"
	}
	var maj, min, build uint32 = 10, 0, 0
	if info := windows.RtlGetVersion(); info != nil {
		maj, min, build = info.MajorVersion, info.MinorVersion, info.BuildNumber
	}

	machine := runtime.GOARCH
	switch machine {
	case "amd64":
		machine = "x86_64"
	case "386":
		machine = "i686"
	case "arm64":
		machine = "aarch64"
	case "arm":
		machine = "armv7l"
	}

	return utsname{
		Sysname:  "Windows_NT",
		Nodename: nodename,
		Release:  fmt.Sprintf("%d.%d.%d", maj, min, build),
		Version:  fmt.Sprintf("build %d", build),
		Machine:  machine,
	}, nil
}
