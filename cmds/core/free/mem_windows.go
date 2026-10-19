// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package free

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type memoryStatusEx struct {
	cbSize               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

func getPlatformMemInfo() (*MemInfo, error) {
	var ms memoryStatusEx
	ms.cbSize = uint32(unsafe.Sizeof(ms))
	r1, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if r1 == 0 {
		return nil, fmt.Errorf("GlobalMemoryStatusEx: %w", err)
	}

	totalMem := ms.totalPhys
	freeMem := ms.availPhys
	var usedMem uint64
	if totalMem >= freeMem {
		usedMem = totalMem - freeMem
	}

	var swapTotal, swapFree, swapUsed uint64
	if ms.totalPageFile > ms.totalPhys {
		swapTotal = ms.totalPageFile - ms.totalPhys
		if ms.availPageFile > ms.availPhys {
			swapFree = ms.availPageFile - ms.availPhys
		}
		if swapTotal >= swapFree {
			swapUsed = swapTotal - swapFree
		}
	}

	return &MemInfo{
		Mem: mainMemInfo{
			Total:     totalMem,
			Used:      usedMem,
			Free:      freeMem,
			Available: freeMem,
		},
		Swap: swapInfo{
			Total: swapTotal,
			Used:  swapUsed,
			Free:  swapFree,
		},
	}, nil
}
