// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !unix && !windows

package tty

import (
	"errors"
	"io"
)

func findTTY(stdin io.Reader) (string, error) {
	return "", errors.New("not a tty")
}
