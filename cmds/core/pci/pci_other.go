// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !windows

package pci

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/u-root/u-root/pkg/core"
)

const usage = "pci [-n] [-v] [-h]"

// Main executes pci on unsupported platforms with a clean non-panicking message.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "pci: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("pci", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var (
		numbers = fs.Bool("n", false, "Show numeric IDs")
		verbose = fs.Bool("v", false, "verbose output")
		help    = fs.Bool("h", false, "show help")
	)

	fs.Usage = func() {
		fmt.Fprintf(ioEnv.Stderr, "Usage: %s\n", usage)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *help {
		fs.Usage()
		return 0
	}

	_ = numbers
	_ = verbose

	fmt.Fprintf(ioEnv.Stderr, "pci: not available on this platform\n")
	return 1
}
