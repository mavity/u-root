// Copyright 2015-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !plan9 && !windows && !wasip1

package uname

import (
	"golang.org/x/sys/unix"
)

func charsToString(ca any) string {
	var buf []byte
	switch b := ca.(type) {
	case []byte:
		buf = b
	case []int8:
		for _, c := range b {
			buf = append(buf, byte(c))
		}
	}
	for i, c := range buf {
		if c == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

func getUtsname() (utsname, error) {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return utsname{}, err
	}
	return utsname{
		Sysname:  charsToString(u.Sysname[:]),
		Nodename: charsToString(u.Nodename[:]),
		Release:  charsToString(u.Release[:]),
		Version:  charsToString(u.Version[:]),
		Machine:  charsToString(u.Machine[:]),
	}, nil
}
