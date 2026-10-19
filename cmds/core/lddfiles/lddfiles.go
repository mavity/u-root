// Copyright 2009-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// lddfiles prints the arguments and all .so dependencies of those arguments
//
// Description:
//
//	lddfiles prints the arguments on the command line and all .so's
//	on which they depend. In some cases, those .so's are actually symlinks;
//	in that case, the symlink and its value are printed.
package lddfiles

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/ldd"
)

func run(stdout io.Writer, args []string) error {
	l, err := ldd.FList(args...)
	if err != nil {
		return fmt.Errorf("ldd: %w", err)
	}

	for _, p := range args {
		a, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("ldd: %w", err)
		}
		l = append(l, a)
	}

	for _, dep := range l {
		fmt.Fprintf(stdout, "%s\n", dep)
	}

	return nil
}

// Main executes the lddfiles command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "lddfiles: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	_ = ctx
	fs := flag.NewFlagSet("lddfiles", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	fs.Usage = func() {
		fmt.Fprintln(ioEnv.Stderr, "Usage: lddfiles [FILE]...")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	// Resolve path arguments relative to ioEnv.Dir
	var resolvedArgs []string
	for _, a := range fs.Args() {
		resolvedArgs = append(resolvedArgs, ioEnv.ResolvePath(a))
	}

	if err := run(ioEnv.Stdout, resolvedArgs); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "lddfiles: %v\n", err)
		return 1
	}
	return 0
}
