// Copyright 2013-2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// readlink display value of symbolic link file.
//
// Synopsis:
//
//	readlink [OPTIONS] [FILE...]
//
// Options:
//
//	-f: follow
//	-n: nonewline
//	-v: verbose
package readlink

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

const cmd = "readlink [-fnv] FILE"

func readLink(ioEnv core.IO, file string, follow, noNewLine bool) error {
	resolved := ioEnv.ResolvePath(file)
	path, err := os.Readlink(resolved)
	if err != nil {
		return err
	}

	if follow {
		path, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			return err
		}
	}

	delimiter := "\n"
	if noNewLine {
		delimiter = ""
	}

	_, err = fmt.Fprintf(ioEnv.Stdout, "%s%s", path, delimiter)
	return err
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var (
		follow    bool
		noNewLine bool
		verbose   bool
	)

	fs := flag.NewFlagSet("readlink", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&follow, "f", false, "follow recursively")
	fs.BoolVar(&noNewLine, "n", false, "do not output trailing newline")
	fs.BoolVar(&verbose, "v", false, "report error messages")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: %s\n", cmd)
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		if verbose {
			fmt.Fprintf(ioEnv.Stderr, "missing operand")
		}
		return fmt.Errorf("missing operand")
	}

	var runErr error
	for _, file := range rest {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := readLink(ioEnv, file, follow, noNewLine)
		if err != nil {
			if verbose {
				fmt.Fprintf(ioEnv.Stderr, "%v\n", err)
			}
			runErr = err
		}
	}

	return runErr
}

// Main runs the readlink command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "readlink: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			// standard readlink exits with code 1 on non-verbose errors as well
		}
		return 1
	}
	return 0
}
