// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package df

import (
	"path/filepath"
	"strings"
)

func deviceNumber(path string) (uint64, error) {
	if path == "" {
		return 0, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	vol := filepath.VolumeName(abs)
	if len(vol) >= 1 {
		ch := strings.ToUpper(vol[:1])[0]
		if ch >= 'A' && ch <= 'Z' {
			return uint64(ch), nil
		}
	}
	return 0, nil
}
