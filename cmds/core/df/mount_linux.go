// Copyright 2015-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package df

import (
	"bytes"
	"math"
	"os"
	"syscall"
)

const procmountsFile = "/proc/mounts"

func getMounts(units uint64) ([]Mount, error) {
	buf, err := os.ReadFile(procmountsFile)
	if err != nil {
		return nil, err
	}
	return mountinfoFromBytes(buf, units)
}

func mountinfoFromBytes(buf []byte, units uint64) ([]Mount, error) {
	var mounts []Mount
	seen := make(map[string]bool)
	for line := range bytes.SplitSeq(buf, []byte{'\n'}) {
		kv := bytes.SplitN(line, []byte{' '}, 6)
		if len(kv) != 6 {
			continue
		}
		mp := string(kv[1])
		if seen[mp] {
			continue
		}
		var mnt Mount
		mnt.Device = string(kv[0])
		mnt.MountPoint = mp
		mnt.FileSystemType = string(kv[2])
		mnt.Flags = string(kv[3])
		if err := diskUsage(&mnt, units); err != nil {
			return nil, err
		}
		if mnt.Blocks == 0 {
			continue
		}
		seen[mp] = true
		mounts = append(mounts, mnt)
	}
	return mounts, nil
}

func diskUsage(mnt *Mount, units uint64) error {
	fs := syscall.Statfs_t{}
	if err := syscall.Statfs(mnt.MountPoint, &fs); err != nil {
		if os.IsPermission(err) {
			return nil
		}
		return err
	}
	if fs.Blocks == 0 {
		return nil
	}
	mnt.Bsize = int64(fs.Bsize)
	mnt.Blocks = fs.Blocks * uint64(fs.Bsize) / units
	mnt.Total = fs.Blocks * uint64(fs.Bsize) / units
	mnt.Avail = uint64(fs.Bavail) * uint64(fs.Bsize) / units
	mnt.Used = (fs.Blocks - fs.Bfree) * uint64(fs.Bsize) / units
	pct := float64((fs.Blocks - fs.Bfree)) * 100 / float64(fs.Blocks)
	mnt.PCT = uint8(math.Ceil(pct))
	return nil
}
