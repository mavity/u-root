// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package realpath

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var quiet bool
	fs := newFlagSet(&quiet, ioEnv.Stderr)

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("missing operand")
	}

	var errs error
	for _, arg := range rest {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		target := ioEnv.ResolvePath(arg)
		absPath, err := filepath.Abs(target)
		if err != nil {
			if !quiet {
				fmt.Fprintf(ioEnv.Stderr, "realpath: %v\n", err)
			}
			errs = errors.Join(errs, err)
			continue
		}

		realPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			if !quiet {
				fmt.Fprintf(ioEnv.Stderr, "realpath: %v\n", err)
			}
			errs = errors.Join(errs, err)
			continue
		}

		fmt.Fprintf(ioEnv.Stdout, "%s\n", filepath.Clean(realPath))
	}

	return errs
}

// Main runs the realpath command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "realpath: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			// error already reported if not quiet
		}
		return 1
	}
	return 0
}

// NewFlagSetForCompletion returns a fresh realpath flag definition.
func NewFlagSetForCompletion() *flag.FlagSet {
	var quiet bool
	return newFlagSet(&quiet, io.Discard)
}

func newFlagSet(quiet *bool, output io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("realpath", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.BoolVar(quiet, "q", false, "quiet mode")
	return fs
}
