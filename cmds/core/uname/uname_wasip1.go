// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip1

package uname

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"syscall"
)

func getUtsname() (utsname, error) {
	platform := syscall.GetPlatformInfo()
	nodename, _ := os.Hostname()
	if nodename == "" || nodename == runtime.GOOS {
		nodename = "rusticated"
	}

	sysname := "Rusticated"
	release := "unknown"
	version := "wasm"
	machine := runtime.GOARCH
	if platform != nil {
		switch strings.ToLower(platform.OSName) {
		case "windows":
			sysname = "Windows_NT"
		case "darwin":
			sysname = "Darwin"
		case "linux":
			sysname = "Linux"
		case "":
		default:
			sysname = platform.OSName
		}

		if platform.OSVersion != [4]uint16{} {
			release = fmt.Sprintf("%d.%d.%d", platform.OSVersion[0], platform.OSVersion[1], platform.OSVersion[2])
		}
		if platform.RusticatedName != "" || platform.RusticatedVersionStr != "" {
			version = strings.TrimSpace(platform.RusticatedName + " " + platform.RusticatedVersionStr)
		}
		if _, arch, ok := strings.Cut(platform.BuildPlatform, "-"); ok && arch != "" {
			machine = normalizeMachine(arch)
		} else {
			switch platform.CPUType {
			case 1:
				machine = "x86_64"
			case 2:
				machine = "aarch64"
			case 3:
				machine = "armv7l"
			}
		}
	}

	return utsname{
		Sysname:  sysname,
		Nodename: nodename,
		Release:  release,
		Version:  version,
		Machine:  machine,
	}, nil
}

func normalizeMachine(machine string) string {
	switch machine {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armv7l"
	default:
		return machine
	}
}
