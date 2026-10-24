// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

//go:build unix

package interp

import "syscall"

func currentUmask() (int, error) {
	mask := syscall.Umask(0)
	syscall.Umask(mask)
	return mask, nil
}

func setUmask(mask int) (int, error) {
	return syscall.Umask(mask), nil
}
