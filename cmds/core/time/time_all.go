// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !tinygo

package timecmd

import (
	"io"
	"os/exec"
)

func printProcessState(w io.Writer, c *exec.Cmd) {
	if c == nil || c.ProcessState == nil {
		printTime(w, "user", 0)
		printTime(w, "sys", 0)
		return
	}
	printTime(w, "user", c.ProcessState.UserTime())
	printTime(w, "sys", c.ProcessState.SystemTime())
}
