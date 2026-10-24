// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

//go:build wasip1

package interp

import "syscall"

func currentUmask() (int, error) {
	return syscall.GetUmask(), nil
}

func setUmask(mask int) (int, error) {
	return syscall.Umask(mask), nil
}
