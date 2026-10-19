// Copyright 2015-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip1

package date

import (
	"syscall"
	"time"
)

func setDate(_ string, _ *time.Location, _ Clock) error {
	return syscall.ENOSYS
}
