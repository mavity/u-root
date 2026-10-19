// Copyright 2012-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Print name of current directory.
//
// Synopsis:
//
//	pwd [-LP]
//
// Options:
//
//	-L: don't follow any symlinks
//	-P: follow all symlinks (avoid all symlinks)
package pwd

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

func pwd(ioEnv core.IO, followSymlinks bool) (string, error) {
	path := ioEnv.Dir
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	if followSymlinks {
		var err error
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
	}
	return path, nil
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	var (
		logical  = true
		physical bool
	)

	fs := flag.NewFlagSet("pwd", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&logical, "L", true, "don't follow any symlinks")
	fs.BoolVar(&physical, "P", false, "follow all symlinks (avoid all symlinks)")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	path, err := pwd(ioEnv, physical)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(ioEnv.Stdout, path)
	return err
}

// Main runs the pwd command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "pwd: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "pwd: %v\n", err)
		}
		return 1
	}
	return 0
}
