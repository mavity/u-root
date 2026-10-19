// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package which locates commands in PATH.
//
// Synopsis:
//
//	which [-a] [-v] [COMMAND]...
//
// Options:
//
//	-a: print all matching pathnames of each argument
//	-v: verbose output
package which

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func findExecutable(p, name string) (string, bool) {
	target := filepath.Join(p, name)
	if canExecute(target) {
		return target, true
	}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		for _, ext := range []string{".exe", ".cmd", ".bat", ".com"} {
			withExt := target + ext
			if canExecute(withExt) {
				return withExt, true
			}
		}
	}
	return "", false
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var allPaths, verbose bool
	fs := flag.NewFlagSet("which", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&allPaths, "a", false, "print all matching pathnames of each argument")
	fs.BoolVar(&verbose, "v", false, "verbose output")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	cmds := fs.Args()
	if len(cmds) == 0 {
		return errors.New("no command specified")
	}

	p, ok := ioEnv.LookupEnv("PATH")
	if !ok || len(p) == 0 {
		p = "/ubin"
	}
	paths := filepath.SplitList(p)
	if len(paths) == 0 {
		paths = []string{"/ubin"}
	}

	var foundAll = true
	for _, name := range cmds {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var foundOne bool
		if ioEnv.IsBuiltin != nil && ioEnv.IsBuiltin(name) {
			foundOne = true
			fmt.Fprintln(ioEnv.Stdout, name)
			if !allPaths {
				continue
			}
		}
		for _, dir := range paths {
			if execPath, ok := findExecutable(dir, name); ok {
				foundOne = true
				fmt.Fprintln(ioEnv.Stdout, execPath)
				if !allPaths {
					break
				}
			}
		}
		if !foundOne {
			foundAll = false
			if verbose {
				fmt.Fprintf(ioEnv.Stderr, "%s: not found\n", name)
			}
		}
	}

	if !foundAll {
		return fmt.Errorf("no suitable executable found")
	}
	return nil
}

// Main runs the which command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "which: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		if !errors.Is(err, flag.ErrHelp) {
			// which typically remains silent or logs with -v
		}
		return 1
	}
	return 0
}
