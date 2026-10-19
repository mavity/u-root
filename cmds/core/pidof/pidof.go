// Copyright 2025 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pidof

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errNotFound = errors.New("pid name not found")

type proc struct {
	comm string
	pid  string
}

func run(stdout io.Writer, procPath string, single, quiet bool, args []string) error {
	procs, err := collect(procPath)
	if err != nil {
		return err
	}

	var pids []string

	for _, proc := range procs {
		for _, arg := range args {
			if proc.comm == arg {
				if single {
					if !quiet {
						fmt.Fprintln(stdout, proc.pid)
					}
					return nil
				}
				pids = append(pids, proc.pid)
			}
		}
	}

	if len(pids) == 0 {
		return errNotFound
	}

	if !quiet {
		fmt.Fprintln(stdout, strings.Join(pids, " "))
	}
	return nil
}

// Main runs the pidof command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "pidof: panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("pidof", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	single := fs.Bool("s", false, "single shot - this instructs the program to only return one pid")
	quiet := fs.Bool("q", false, "do not display matched PIDs to standard out")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if len(fs.Args()) == 0 {
		return 1
	}

	if err := run(ioEnv.Stdout, procPath, *single, *quiet, fs.Args()); err != nil {
		if errors.Is(err, errNotFound) {
			return 1
		}
		fmt.Fprintf(ioEnv.Stderr, "pidof: %v\n", err)
		return 1
	}
	return 0
}
