// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip1

package cpio

import "syscall"

func sysInfo(n string, sys *syscall.Stat_t) Info {
	return Info{
		Ino:      sys.Ino,
		Mode:     uint64(sys.Mode),
		UID:      uint64(sys.Uid),
		GID:      uint64(sys.Gid),
		NLink:    uint64(sys.Nlink),
		MTime:    uint64(sys.Mtime / 1_000_000_000),
		FileSize: uint64(sys.Size),
		Dev:      uint64(sys.Dev),
		Major:    uint64(sys.Dev >> 8),
		Minor:    uint64(sys.Dev & 0xff),
		Rmajor:   0,
		Rminor:   0,
		Name:     n,
	}
}
