// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package df

import (
	"math"
	"syscall"

	"golang.org/x/sys/windows"
)

func getMounts(units uint64) ([]Mount, error) {
	drivesMask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, err
	}

	var mounts []Mount
	for i := 0; i < 26; i++ {
		if (drivesMask & (1 << i)) == 0 {
			continue
		}
		letter := string(rune('A' + i))
		rootPath := letter + `:\`

		dt := windows.GetDriveType(windows.StringToUTF16Ptr(rootPath))
		if dt == windows.DRIVE_NO_ROOT_DIR || dt == windows.DRIVE_UNKNOWN {
			continue
		}

		var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
		err = windows.GetDiskFreeSpaceEx(
			windows.StringToUTF16Ptr(rootPath),
			&freeBytesAvailable,
			&totalNumberOfBytes,
			&totalNumberOfFreeBytes,
		)
		if err != nil || totalNumberOfBytes == 0 {
			continue
		}

		var fsNameBuf [256]uint16
		var volNameBuf [256]uint16
		var maxComponentLen, fsFlags uint32
		_ = windows.GetVolumeInformation(
			windows.StringToUTF16Ptr(rootPath),
			&volNameBuf[0],
			uint32(len(volNameBuf)),
			nil,
			&maxComponentLen,
			&fsFlags,
			&fsNameBuf[0],
			uint32(len(fsNameBuf)),
		)
		fsType := syscall.UTF16ToString(fsNameBuf[:])
		if fsType == "" {
			fsType = "NTFS"
		}

		total := totalNumberOfBytes / units
		avail := freeBytesAvailable / units
		used := (totalNumberOfBytes - totalNumberOfFreeBytes) / units
		var pct uint8
		if totalNumberOfBytes > 0 {
			pct = uint8(math.Ceil(float64(totalNumberOfBytes-totalNumberOfFreeBytes) * 100 / float64(totalNumberOfBytes)))
		}

		mounts = append(mounts, Mount{
			Device:         letter + ":",
			MountPoint:     rootPath,
			FileSystemType: fsType,
			Blocks:         total,
			Total:          total,
			Used:           used,
			Avail:          avail,
			PCT:            pct,
		})
	}
	return mounts, nil
}
