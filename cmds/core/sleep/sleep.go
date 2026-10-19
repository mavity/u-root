// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sleep delays for the specified amount of time.
//
// Synopsis:
//
//	sleep DURATION
//
// Description:
//
//	If no units are given, the duration is assumed to be measured in
//	seconds, otherwise any format parsed by Go's `time.ParseDuration` is
//	accepted.
package sleep

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errDuration = errors.New("invalid duration")

func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		d, err = time.ParseDuration(s + "s")
	}
	if err != nil || d < 0 {
		return time.Duration(0), errDuration
	}
	return d, nil
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	fs := flag.NewFlagSet("sleep", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return errors.New("incorrect number of arguments")
	}

	d, err := parseDuration(fs.Arg(0))
	if err != nil {
		return err
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Main runs the sleep command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "sleep: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "sleep: %v\n", err)
		}
		return 1
	}
	return 0
}
