// Copyright 2012-2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package timecmd measures process execution duration.
//
// Synopsis:
//
//	time [-p] CMD [ARG]...
//
// Description:
//
//	After executing CMD, its real, user and system times are printed to
//	stderr in the POSIX format.
package timecmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func printTime(stderr io.Writer, l string, t time.Duration) {
	fmt.Fprintf(stderr, "%s\n", label(l, t))
}

func label(l string, t time.Duration) string {
	return fmt.Sprintf("%s %.03f", l, t.Seconds())
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var posix bool
	fs := flag.NewFlagSet("time", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&posix, "p", true, "makes time output POSIX.2 compliant")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	cmdArgs := fs.Args()
	start := time.Now()
	if len(cmdArgs) == 0 {
		fmt.Fprintf(ioEnv.Stderr, "%s\n", strings.Join([]string{
			label("real", 0*time.Second),
			label("user", 0*time.Second),
			label("sys", 0*time.Second),
		}, "\n"))
		return nil
	}

	if ioEnv.Runner != nil {
		exitCode := ioEnv.Runner(ctx, ioEnv, cmdArgs...)
		realTime := time.Since(start)
		printTime(ioEnv.Stderr, "real", realTime)
		printTime(ioEnv.Stderr, "user", 0)
		printTime(ioEnv.Stderr, "sys", 0)
		if exitCode != 0 {
			return fmt.Errorf("%q exited with status %d", cmdArgs, exitCode)
		}
		return nil
	}

	c := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	c.Stdin = ioEnv.Stdin
	c.Stdout = ioEnv.Stdout
	c.Stderr = ioEnv.Stderr
	defer func(*exec.Cmd, time.Time) {
		realTime := time.Since(start)
		printTime(ioEnv.Stderr, "real", realTime)
		printProcessState(ioEnv.Stderr, c)
	}(c, start)

	if err := c.Run(); err != nil {
		return fmt.Errorf("%q:%w", cmdArgs, err)
	}
	return nil
}

// Main runs the time command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "time: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "time: %v\n", err)
		}
		return 1
	}
	return 0
}
