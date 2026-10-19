// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !darwin && !freebsd && !linux

package timeout

import "syscall"

var sigmap = map[string]syscall.Signal{
	"KILL": syscall.Signal(9),
	"TERM": syscall.Signal(15),
	"INT":  syscall.Signal(2),
	"HUP":  syscall.Signal(1),
}
