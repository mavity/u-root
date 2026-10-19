// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package ps

import (
	"os"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func loadPlatformProcesses(x bool) ([]*Process, *Process, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, nil, err
	}
	defer windows.CloseHandle(snapshot)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	err = windows.Process32First(snapshot, &pe)
	if err != nil {
		return nil, nil, err
	}

	currentPID := os.Getpid()
	var procs []*Process
	var mProc *Process

	for {
		pid := int(pe.ProcessID)
		ppid := int(pe.ParentProcessID)
		name := syscall.UTF16ToString(pe.ExeFile[:])

		p := &Process{
			process: process{
				Pid:   strconv.Itoa(pid),
				Cmd:   name,
				State: "R",
				Ppid:  strconv.Itoa(ppid),
				Pgrp:  strconv.Itoa(ppid),
				Sid:   "0",
				Ctty:  "?",
				Time:  "00:00:00",
			},
			Pidno:   pid,
			uid:     os.Geteuid(),
			cmdline: name,
		}

		if pid == currentPID {
			mProc = p
		}
		procs = append(procs, p)

		err = windows.Process32Next(snapshot, &pe)
		if err != nil {
			break
		}
	}

	if mProc == nil && len(procs) > 0 {
		mProc = procs[0]
	}

	return procs, mProc, nil
}
