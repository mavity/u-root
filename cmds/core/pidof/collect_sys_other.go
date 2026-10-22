// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !windows

package pidof

import (
	"os"
	"runtime"
	"strconv"
)

func collectSystemProcesses() ([]proc, error) {
	if _, err := os.Stat(procPath); err == nil {
		return collectDir(procPath)
	}
	pid := strconv.Itoa(os.Getpid())
	return []proc{
		{comm: fallbackProcessName(runtime.GOOS, os.Args[0]), pid: pid},
	}, nil
}
