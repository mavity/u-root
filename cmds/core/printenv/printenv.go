// Copyright 2014-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package printenv prints environment variables.
//
// Synopsis:
//
//	printenv [-0] [VARIABLE]...
package printenv

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var null bool
	fs := flag.NewFlagSet("printenv", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&null, "0", false, "end each output line with NUL, not newline")
	fs.BoolVar(&null, "null", false, "end each output line with NUL, not newline")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: printenv [-0] [VARIABLE]...\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	delimiter := "\n"
	if null {
		delimiter = "\x00"
	}

	rest := fs.Args()
	if len(rest) == 0 {
		var envs []string
		if ioEnv.Environ != nil {
			envs = ioEnv.Environ()
		} else {
			envs = os.Environ()
		}

		for _, v := range envs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			fmt.Fprintf(ioEnv.Stdout, "%s%s", v, delimiter)
		}
		return nil
	}

	var foundAll = true
	for _, arg := range rest {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var val string
		var ok bool
		if ioEnv.LookupEnv != nil {
			val, ok = ioEnv.LookupEnv(arg)
		} else {
			val, ok = os.LookupEnv(arg)
		}

		if ok {
			fmt.Fprintf(ioEnv.Stdout, "%s%s", val, delimiter)
		} else {
			foundAll = false
		}
	}

	if !foundAll {
		return fmt.Errorf("variable not found")
	}
	return nil
}

// Main runs the printenv command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "printenv: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			// printenv quietly returns exit code 1 when a variable is not found
		}
		return 1
	}
	return 0
}
