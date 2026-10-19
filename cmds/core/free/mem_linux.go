// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package free

import (
	"os"
)

const meminfoFile = "/proc/meminfo"

func getPlatformMemInfo() (*MemInfo, error) {
	buf, err := os.ReadFile(meminfoFile)
	if err != nil {
		return nil, err
	}
	m, err := meminfoFromBytes(buf)
	if err != nil {
		return nil, err
	}
	mmi, err := getMainMemInfo(m)
	if err != nil {
		return nil, err
	}
	si, err := getSwapInfo(m)
	if err != nil {
		return nil, err
	}
	return &MemInfo{Mem: *mmi, Swap: *si}, nil
}
