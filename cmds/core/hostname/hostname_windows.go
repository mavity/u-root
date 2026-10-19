// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package hostname

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procSetComputerNameExW = modkernel32.NewProc("SetComputerNameExW")
)

const computerNamePhysicalDnsHostname = 5

func Sethostname(n string) error {
	ptr, err := syscall.UTF16PtrFromString(n)
	if err != nil {
		return err
	}
	r1, _, errSys := procSetComputerNameExW.Call(
		uintptr(computerNamePhysicalDnsHostname),
		uintptr(unsafe.Pointer(ptr)),
	)
	if r1 == 0 {
		return errSys
	}
	return nil
}
