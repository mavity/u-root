// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package pidof

import (
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func collectSystemProcesses() ([]proc, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	err = windows.Process32First(snapshot, &pe)
	if err != nil {
		return nil, err
	}

	var procs []proc
	for {
		pid := strconv.Itoa(int(pe.ProcessID))
		name := syscall.UTF16ToString(pe.ExeFile[:])
		procs = append(procs, proc{comm: name, pid: pid})

		if strings.HasSuffix(strings.ToLower(name), ".exe") {
			base := name[:len(name)-4]
			procs = append(procs, proc{comm: base, pid: pid})
		}

		err = windows.Process32Next(snapshot, &pe)
		if err != nil {
			break
		}
	}
	return procs, nil
}
