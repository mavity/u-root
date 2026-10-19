// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !windows && !darwin

package free

func getPlatformMemInfo() (*MemInfo, error) {
	total := uint64(1024 * 1024 * 1024) // 1 GiB
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
