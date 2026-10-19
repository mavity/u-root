// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpio

import (
	"syscall"
)

func sysInfo(n string, sys *syscall.Win32FileAttributeData) Info {
	sz := uint64(sys.FileSizeHigh)<<32 | uint64(sys.FileSizeLow)
	mtime := uint64(sys.LastWriteTime.Nanoseconds()) / 1_000_000_000

	mode := uint64(modeFile | 0o644)
	if sys.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		mode = uint64(modeDir | 0o755)
	}

	return Info{
		Ino:      0,
		Mode:     mode,
		UID:      uint64(0),
		GID:      uint64(0),
		NLink:    uint64(1),
		MTime:    mtime,
		FileSize: sz,
		Dev:      0,
		Major:    0,
		Minor:    0,
		Rmajor:   0,
		Rminor:   0,
		Name:     n,
	}
}
