// Copyright 2012-2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package timeout runs a command and cancels it if it runs more than a specified duration.
package timeout

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errNoArgs = errors.New("need at least a command to run")

type cmd struct {
	args     []string
	timeout  time.Duration
	signal   string
	in       io.Reader
	out, err io.Writer
}

func (c *cmd) run() (int, error) {
	if len(c.args) == 0 {
		return 1, errNoArgs
	}

	if c.signal != "" {
		if _, ok := sigmap[c.signal]; !ok {
			return 1, fmt.Errorf("unknown signal: %q: %w", c.signal, os.ErrInvalid)
		}
	}

	timeoutVal := c.timeout
	if timeoutVal <= 0 {
		timeoutVal = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutVal)
	defer cancel()

	proc := exec.CommandContext(ctx, c.args[0], c.args[1:]...)
	proc.Stdin, proc.Stdout, proc.Stderr = c.in, c.out, c.err
	if err := proc.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return 124, ctx.Err()
		}
		var e *exec.ExitError
		if errors.As(err, &e) {
			return e.ExitCode(), err
		}
		return 1, err
	}
	return 0, nil
}

func runCommand(ctx context.Context, ioEnv core.IO, timeoutVal time.Duration, sigName string, cmdArgs []string) (int, error) {
	if len(cmdArgs) == 0 {
		return 1, errNoArgs
	}
	if sigName != "" {
		if _, ok := sigmap[sigName]; !ok {
			return 1, fmt.Errorf("unknown signal: %q: %w", sigName, os.ErrInvalid)
		}
	}

	if timeoutVal <= 0 {
		timeoutVal = 30 * time.Second
	}

	runCtx, cancel := context.WithTimeout(ctx, timeoutVal)
	defer cancel()

	if ioEnv.Runner != nil {
		code := ioEnv.Runner(runCtx, ioEnv, cmdArgs...)
		if runCtx.Err() == context.DeadlineExceeded {
			return 124, runCtx.Err()
		}
		if code != 0 {
			return code, fmt.Errorf("command exited with code %d", code)
		}
		return 0, nil
	}

	proc := exec.CommandContext(runCtx, cmdArgs[0], cmdArgs[1:]...)
	proc.Stdin = ioEnv.Stdin
	proc.Stdout = ioEnv.Stdout
	proc.Stderr = ioEnv.Stderr
	if err := proc.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return 124, runCtx.Err()
		}
		var e *exec.ExitError
		if errors.As(err, &e) {
			return e.ExitCode(), err
		}
		return 1, err
	}
	return 0, nil
}

// Main runs the timeout command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "timeout: panic: %v\n", r)
			exitCode = 1
		}
	}()

	var timeoutVal time.Duration
	var sigName string
	fs := flag.NewFlagSet("timeout", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.DurationVar(&timeoutVal, "t", 0, "Timeout for command")
	fs.StringVar(&sigName, "signal", "TERM", "specify the signal to be sent on timeout")
	fs.StringVar(&sigName, "s", "TERM", "specify the signal to be sent on timeout")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "timeout: %v\n", err)
		}
		return 1
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintf(ioEnv.Stderr, "timeout: %v\n", errNoArgs)
		return 1
	}

	if timeoutVal == 0 {
		if d, err := time.ParseDuration(rest[0]); err == nil && d > 0 {
			timeoutVal = d
			rest = rest[1:]
		} else if d, err := time.ParseDuration(rest[0] + "s"); err == nil && d > 0 {
			timeoutVal = d
			rest = rest[1:]
		}
	}

	if len(rest) == 0 {
		fmt.Fprintf(ioEnv.Stderr, "timeout: %v\n", errNoArgs)
		return 1
	}

	code, err := runCommand(ctx, ioEnv, timeoutVal, sigName, rest)
	if err != nil && code == 0 {
		code = 1
	}
	return code
}
