// Copyright 2012-2025 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package backoff runs a command, repeatedly, until it succeeds or we are out of time.
//
// Synopsis:
//
//	backoff [-v] [-t duration-string] command [args...]
//
// Description:
//
//	backoff will run the command until it succeeds or a timeout has passed.
//	The default timeout is 30s.
package backoff

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"time"

	cenkaltibackoff "github.com/cenkalti/backoff/v4"
	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errNoCmd = errors.New("no command passed")

func run(ctx context.Context, ioEnv core.IO, timeoutVal time.Duration, verbose bool, cmdArgs ...string) error {
	if len(cmdArgs) == 0 || cmdArgs[0] == "" {
		return errNoCmd
	}

	b := cenkaltibackoff.NewExponentialBackOff()
	b.MaxElapsedTime = timeoutVal
	bCtx := cenkaltibackoff.WithContext(b, ctx)

	f := func() error {
		select {
		case <-ctx.Done():
			return cenkaltibackoff.Permanent(ctx.Err())
		default:
		}

		if ioEnv.Runner != nil {
			code := ioEnv.Runner(ctx, ioEnv, cmdArgs...)
			if code != 0 {
				err := fmt.Errorf("command %s exited with status %d", cmdArgs[0], code)
				if verbose {
					fmt.Fprintf(ioEnv.Stderr, "%q %v: %v\n", cmdArgs[0], cmdArgs[1:], err)
				}
				return err
			}
			return nil
		}

		cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
		cmd.Stdin = ioEnv.Stdin
		cmd.Stdout = ioEnv.Stdout
		cmd.Stderr = ioEnv.Stderr
		err := cmd.Run()
		if verbose {
			fmt.Fprintf(ioEnv.Stderr, "%q %v: %v\n", cmdArgs[0], cmdArgs[1:], err)
		}
		return err
	}

	return cenkaltibackoff.Retry(f, bCtx)
}

// Main runs the backoff command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "backoff: panic: %v\n", r)
			exitCode = 1
		}
	}()

	var timeoutVal time.Duration
	var verbose bool
	fs := flag.NewFlagSet("backoff", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.DurationVar(&timeoutVal, "t", 30*time.Second, "Timeout for command")
	fs.BoolVar(&verbose, "v", false, "Log each attempt to run the command")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "backoff: %v\n", err)
		}
		return 1
	}

	cmdArgs := fs.Args()
	if len(cmdArgs) == 0 {
		fs.Usage()
		return 1
	}

	if err := run(ctx, ioEnv, timeoutVal, verbose, cmdArgs...); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "backoff: %v\n", err)
		return 1
	}
	return 0
}
