// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin

package free

import (
	"golang.org/x/sys/unix"
)

func getPlatformMemInfo() (*MemInfo, error) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return nil, err
	}
	free := total / 2
	used := total - free
	return &MemInfo{
		Mem: mainMemInfo{
			Total:     total,
			Used:      used,
			Free:      free,
			Available: free,
		},
		Swap: swapInfo{},
	}, nil
}
