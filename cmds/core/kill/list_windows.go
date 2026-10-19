// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package kill

import (
	"fmt"
	"os"
	"syscall"
)

var (
	signames = []string{
		"SIGHUP",
		"SIGINT",
		"SIGQUIT",
		"SIGILL",
		"SIGTRAP",
		"SIGABRT",
		"SIGBUS",
		"SIGFPE",
		"SIGKILL",
		"SIGUSR1",
		"SIGSEGV",
		"SIGUSR2",
		"SIGPIPE",
		"SIGALRM",
		"SIGTERM",
		"SIGSTKFLT",
		"SIGCHLD",
		"SIGCONT",
		"SIGSTOP",
		"SIGTSTP",
		"SIGTTIN",
		"SIGTTOU",
		"SIGURG",
		"SIGXCPU",
		"SIGXFSZ",
		"SIGVTALRM",
		"SIGPROF",
		"SIGWINCH",
		"SIGIO",
		"SIGPWR",
		"SIGSYS",
	}

	signums = func() map[string]os.Signal {
		m := make(map[string]os.Signal)
		for i, name := range signames {
			sigNum := syscall.Signal(i + 1)
			m[name] = sigNum
			m[fmt.Sprintf("%d", i+1)] = sigNum
		}
		return m
	}()
)
