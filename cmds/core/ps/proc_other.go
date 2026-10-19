// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !windows && !darwin

package ps

import (
	"os"
	"strconv"
)

func loadPlatformProcesses(x bool) ([]*Process, *Process, error) {
	currentPID := os.Getpid()
	p := &Process{
		process: process{
			Pid:   strconv.Itoa(currentPID),
			Cmd:   "rusticated",
			State: "R",
			Ctty:  "?",
			Time:  "00:00:00",
		},
		Pidno: currentPID,
		uid:   os.Geteuid(),
	}
	return []*Process{p}, p, nil
}
