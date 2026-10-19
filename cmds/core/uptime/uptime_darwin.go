// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin

package uptime

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

func getSystemUptimeAndLoad() (time.Duration, string, error) {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return 0, "", err
	}
	bootTime := time.Unix(tv.Sec, int64(tv.Usec)*1000)
	d := time.Since(bootTime)

	loadavg, err := unix.SysctlLoadavg("vm.loadavg")
	var loadStr string
	if err == nil && len(loadavg.Load) >= 3 {
		scale := float64(loadavg.Scale)
		if scale <= 0 {
			scale = 1
		}
		loadStr = fmt.Sprintf("%.2f, %.2f, %.2f",
			float64(loadavg.Load[0])/scale,
			float64(loadavg.Load[1])/scale,
			float64(loadavg.Load[2])/scale)
	} else {
		loadStr = "0.00, 0.00, 0.00"
	}
	return d, loadStr, nil
}
