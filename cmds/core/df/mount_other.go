// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !darwin && !windows

package df

func getMounts(units uint64) ([]Mount, error) {
	return []Mount{
		{
			Device:         "/dev/root",
			MountPoint:     "/",
			FileSystemType: "wasi",
			Blocks:         1048576,
			Total:          1048576,
			Used:           1024,
			Avail:          1047552,
			PCT:            1,
		},
	}, nil
}
