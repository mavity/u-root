// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip1

package ls

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"

	humanize "github.com/dustin/go-humanize"
)

// FileInfo holds file metadata for wasip1.
type FileInfo struct {
	Name          string
	Mode          os.FileMode
	Rdev          uint64
	UID, GID      uint32
	Size          int64
	BlkSize       int64
	Blocks        int64
	ATime         time.Time
	MTime         time.Time
	CTime         time.Time
	BirthTime     time.Time
	SymlinkTarget string
	Dev           uint64
	Ino           uint64
	NLink         uint64
}

// FromOSFileInfo converts os.FileInfo to an ls.FileInfo for wasip1.
func FromOSFileInfo(path string, fi os.FileInfo) FileInfo {
	res := FileInfo{
		Name:  fi.Name(),
		Mode:  fi.Mode(),
		Size:  fi.Size(),
		MTime: fi.ModTime(),
	}

	if target, err := os.Readlink(path); err == nil {
		res.SymlinkTarget = target
	}

	if sys, ok := fi.Sys().(*syscall.Stat_t); ok && sys != nil {
		res.Dev = sys.Dev
		res.Ino = sys.Ino
		res.NLink = sys.Nlink
		res.ATime = time.Unix(0, int64(sys.Atime))
		res.CTime = time.Unix(0, int64(sys.Ctime))
	}

	return res
}

// FileString implements Stringer.FileString for wasip1.
func (ls LongStringer) FileString(fi FileInfo) string {
	var size string
	if ls.Human {
		size = humanize.Bytes(uint64(fi.Size))
	} else {
		size = strconv.FormatInt(fi.Size, 10)
	}

	res := fmt.Sprintf("%s\t%d\t%d\t%s\t%v\t%s",
		fi.Mode.String(),
		fi.UID,
		fi.GID,
		size,
		fi.MTime.Format("Jan _2 15:04"),
		ls.Name.FileString(fi),
	)

	if fi.SymlinkTarget != "" {
		res = fmt.Sprintf("%s -> %s", res, fi.SymlinkTarget)
	}
	return res
}
