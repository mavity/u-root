// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package uptime

import (
	"time"

	"golang.org/x/sys/windows"
)

var (
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount64 = kernel32.NewProc("GetTickCount64")
)

func getSystemUptimeAndLoad() (time.Duration, string, error) {
	r1, _, _ := procGetTickCount64.Call()
	millis := int64(r1)
	d := time.Duration(millis) * time.Millisecond
	return d, "0.00, 0.00, 0.00", nil
}
