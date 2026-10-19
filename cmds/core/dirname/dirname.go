// Copyright 2016-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dirname prints out the directory name of one or more arguments.
package dirname

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var ErrNoArg = errors.New("missing operand")

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var zero bool
	fs := flag.NewFlagSet("dirname", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&zero, "z", false, "end each output line with NUL, not newline")
	fs.BoolVar(&zero, "zero", false, "end each output line with NUL, not newline")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: dirname [OPTION] NAME...\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) < 1 {
		return ErrNoArg
	}

	delimiter := "\n"
	if zero {
		delimiter = "\x00"
	}

	for _, n := range rest {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Fprintf(ioEnv.Stdout, "%s%s", filepath.Dir(n), delimiter)
	}
	return nil
}

// Main runs the dirname command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "dirname: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "dirname: %v\n", err)
		}
		return 1
	}
	return 0
}
