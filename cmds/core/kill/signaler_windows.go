// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package kill

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

var defaultSignal = "-SIGTERM"

func kill(sig os.Signal, pids ...string) error {
	var errs error
	for _, p := range pids {
		pid, err := strconv.Atoi(p)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("%v: arguments must be process or job IDS", p))
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if err := proc.Kill(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}
