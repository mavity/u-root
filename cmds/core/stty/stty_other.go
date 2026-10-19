// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (!unix || wasip1) && !windows

package stty

import (
	"fmt"
	"io"

	"github.com/u-root/u-root/pkg/core"
)

func run(stdout, stderr io.Writer, ioEnv core.IO, args []string) error {
	fmt.Fprintf(stdout, "rows:24 cols:80 speed:38400\n")
	return nil
}
