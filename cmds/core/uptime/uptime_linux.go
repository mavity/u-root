// Copyright 2013-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package uptime

import (
	"os"
	"time"
)

func getSystemUptimeAndLoad() (time.Duration, string, error) {
	procUptimeOutput, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, "", err
	}
	uptimeTime, err := uptime(string(procUptimeOutput))
	if err != nil {
		return 0, "", err
	}
	d := uptimeTime.Sub(time.Time{})

	procLoadAvgOutput, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return d, "", err
	}
	loadAverage, err := loadavg(string(procLoadAvgOutput))
	if err != nil {
		return d, "", err
	}
	return d, loadAverage, nil
}
