// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

//go:build !unix && !wasip1

package interp

import "fmt"

func currentUmask() (int, error) {
	return 0, fmt.Errorf("umask is unsupported on this platform")
}

func setUmask(int) (int, error) {
	return 0, fmt.Errorf("umask is unsupported on this platform")
}
