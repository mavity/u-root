// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin

package df

import (
	"math"

	"golang.org/x/sys/unix"
)

func getMounts(units uint64) ([]Mount, error) {
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	buf := make([]unix.Statfs_t, n)
	_, err = unix.Getfsstat(buf, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}

	var mounts []Mount
	for _, fs := range buf {
		if fs.Blocks == 0 {
			continue
		}
		var mnt Mount
		mnt.Device = unix.ByteSliceToString(fs.Mntfromname[:])
		mnt.MountPoint = unix.ByteSliceToString(fs.Mntonname[:])
		mnt.FileSystemType = unix.ByteSliceToString(fs.Fstypename[:])
		mnt.Bsize = int64(fs.Bsize)
		mnt.Blocks = fs.Blocks * uint64(fs.Bsize) / units
		mnt.Total = mnt.Blocks
		mnt.Avail = uint64(fs.Bavail) * uint64(fs.Bsize) / units
		mnt.Used = (fs.Blocks - fs.Bfree) * uint64(fs.Bsize) / units
		pct := float64((fs.Blocks - fs.Bfree)) * 100 / float64(fs.Blocks)
		mnt.PCT = uint8(math.Ceil(pct))
		mounts = append(mounts, mnt)
	}
	return mounts, nil
}
