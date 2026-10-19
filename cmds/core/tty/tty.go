// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// tty prints the file name of the terminal connected to standard input.
//
// Synopsis:
//
//	tty [-s]
package tty

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func run(stdout io.Writer, stdin io.Reader, silent bool) error {
	path, err := findTTY(stdin)
	if err != nil {
		if !silent {
			fmt.Fprintln(stdout, "not a tty")
		}
		return err
	}
	if !silent {
		fmt.Fprintln(stdout, path)
	}
	return nil
}

// Main runs the tty command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "tty: panic: %v\n", r)
			exitCode = 3
		}
	}()

	fs := flag.NewFlagSet("tty", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	silent := fs.Bool("s", false, "print nothing, only return an exit status")
	fs.BoolVar(silent, "silent", false, "print nothing, only return an exit status")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := run(ioEnv.Stdout, ioEnv.Stdin, *silent); err != nil {
		return 1
	}
	return 0
}
