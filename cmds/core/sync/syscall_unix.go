// Copyright 2016-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package sync

import (
	"os"
	"syscall"
)

func doSyscall(syscallNum uintptr, args []string) error {
	for _, fileName := range args {
		f, err := os.OpenFile(fileName, syscall.O_RDONLY|syscall.O_CLOEXEC, 0o644)
		if err != nil {
			return err
		}
		if _, _, err = syscall.Syscall(syscallNum, uintptr(f.Fd()), 0, 0); err.(syscall.Errno) != 0 {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}
