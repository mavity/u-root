// Copyright 2016-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// sync command in Go.
//
// Synopsis:
//
//	sync [-df] [FILE]...
package sync

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var usage = "Usage: sync [-df] [FILE]...\n"

// Main executes the sync command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "sync: panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	data := fs.Bool("data", false, "sync file data, no metadata")
	fs.BoolVar(data, "d", false, "sync file data, no metadata")
	filesystem := fs.Bool("filesystem", false, "commit filesystem caches to disk")
	fs.BoolVar(filesystem, "f", false, "commit filesystem caches to disk")

	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	resolvedArgs := make([]string, len(fs.Args()))
	for i, arg := range fs.Args() {
		resolvedArgs[i] = ioEnv.ResolvePath(arg)
	}

	if err := sync(*data, *filesystem, resolvedArgs); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "sync: %v\n", err)
		return 1
	}
	return 0
}


