// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dmesg

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/u-root/u-root/pkg/core"
)

// Main executes the dmesg command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "dmesg: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	_ = ctx
	fs := flag.NewFlagSet("dmesg", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var clr, readClr bool
	fs.BoolVar(&clr, "clear", false, "Clear the log")
	fs.BoolVar(&clr, "c", false, "Clear the log (shorthand)")
	fs.BoolVar(&readClr, "read-clear", false, "Clear the log after printing")
	fs.BoolVar(&readClr, "r", false, "Clear the log after printing (shorthand)")

	fs.Usage = func() {
		fmt.Fprintln(ioEnv.Stderr, "Usage: dmesg [-clear|-read-clear]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if err := runDmesg(ioEnv.Stdout, clr, readClr); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "dmesg: %v\n", err)
		return 1
	}
	return 0
}
