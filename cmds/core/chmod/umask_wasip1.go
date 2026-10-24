// Copyright 2016-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip1

package chmod

import "syscall"

func currentGuestUmask() (int, error) {
	return syscall.GetUmask(), nil
}
