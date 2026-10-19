// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !windows && !darwin

package uptime

import (
	"os"
	"time"
)

func getSystemUptimeAndLoad() (time.Duration, string, error) {
	if buf, err := os.ReadFile("/proc/uptime"); err == nil {
		if t, err := uptime(string(buf)); err == nil {
			loadStr := "0.00, 0.00, 0.00"
			if lbuf, err := os.ReadFile("/proc/loadavg"); err == nil {
				if l, err := loadavg(string(lbuf)); err == nil {
					loadStr = l
				}
			}
			return t.Sub(time.Time{}), loadStr, nil
		}
	}
	return 1 * time.Hour, "0.00, 0.00, 0.00", nil
}
