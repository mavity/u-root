// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Delete files.
//
// Synopsis:
//
//	rm [-Rrvif] FILE...
//
// Options:
//
//	-i: interactive mode
//	-v: verbose mode
//	-R: remove file hierarchies
//	-r: equivalent to -R
//	-f: ignore nonexistent files and never prompt
package rm

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errUsage = errors.New("usage: rm [-Rrvif] file")

type flags struct {
	interactive bool
	verbose     bool
	recursive   bool
	force       bool
}

func rm(ctx context.Context, ioEnv core.IO, args ...string) error {
	var f flags
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	fs.BoolVar(&f.interactive, "i", false, "Interactive mode.")
	fs.BoolVar(&f.verbose, "v", false, "Verbose mode.")
	fs.BoolVar(&f.recursive, "r", false, "equivalent to -R")
	fs.BoolVar(&f.recursive, "R", false, "Recursive, remove hierarchies")
	fs.BoolVar(&f.force, "f", false, "Force, ignore nonexistent files and never prompt")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "%s...\n", errUsage)
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	files := fs.Args()

	if len(files) < 1 {
		return errUsage
	}

	rmf := os.Remove
	if f.recursive {
		rmf = os.RemoveAll
	}

	if f.force {
		f.interactive = false
	}

	input := bufio.NewReader(ioEnv.Stdin)
	for _, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if f.interactive {
			fmt.Fprintf(ioEnv.Stdout, "rm: remove '%v'? ", file)
			answer, err := input.ReadString('\n')
			if err != nil || len(answer) == 0 || (answer[0] != 'y' && answer[0] != 'Y') {
				continue
			}
		}

		resolvedFile := ioEnv.ResolvePath(file)
		if err := rmf(resolvedFile); err != nil {
			if f.force && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}

		if f.verbose {
			fmt.Fprintf(ioEnv.Stdout, "removed %q\n", file)
		}
	}
	return nil
}

// Main runs the rm command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "rm: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := rm(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "rm: %v\n", err)
		}
		return 1
	}
	return 0
}
