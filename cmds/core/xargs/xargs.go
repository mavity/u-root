// Copyright 2013-2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package xargs reads space, tab, newline and end-of-file delimited strings from the
// standard input and executes utility with the strings as arguments.
//
// Synopsis:
//
//	xargs [OPTIONS] [COMMAND [ARGS]...]
//
// Options:
//
//	-n: max number of arguments per command
//	-t: enable trace mode, each command is written to stderr
//	-p: the user is asked whether to execute utility at each invocation
//	-0: use a null byte as the input argument delimiter
package xargs

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/u-root/u-root/cmds/core/echo"
	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

const defaultMaxArgs = 5000
const defaultTTY = "/dev/tty"

var errTooSmall = errors.New("too small")

type params struct {
	maxArgs int
	trace   bool
	prompt  bool
	null    bool
	tty     string
}

func executeCmd(ctx context.Context, ioEnv core.IO, cmdArgs []string) error {
	if ioEnv.Runner != nil {
		code := ioEnv.Runner(ctx, ioEnv, cmdArgs...)
		if code != 0 {
			return fmt.Errorf("command %s exited with status %d", cmdArgs[0], code)
		}
		return nil
	}
	if cmdArgs[0] == "echo" {
		code := echo.Main(ctx, ioEnv, cmdArgs[1:]...)
		if code != 0 {
			return fmt.Errorf("echo exited with status %d", code)
		}
		return nil
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = ioEnv.Stdin
	cmd.Stdout = ioEnv.Stdout
	cmd.Stderr = ioEnv.Stderr
	return cmd.Run()
}

func run(ctx context.Context, ioEnv core.IO, p params, args ...string) error {
	if p.maxArgs <= 0 {
		return fmt.Errorf("-n %d: %w", p.maxArgs, errTooSmall)
	}

	if len(args) == 0 {
		args = append(args, "echo")
	}

	var xArgs []string

	if p.null {
		r := bufio.NewReader(ioEnv.Stdin)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			b, err := r.ReadBytes(0x00)
			if err != nil && err != io.EOF {
				return err
			}
			if len(b) != 0 {
				if b[len(b)-1] == 0x00 {
					xArgs = append(xArgs, string(b[:len(b)-1]))
				} else {
					xArgs = append(xArgs, string(b))
				}
			}
			if err == io.EOF {
				break
			}
		}
	} else {
		scanner := bufio.NewScanner(ioEnv.Stdin)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			sp := strings.Fields(scanner.Text())
			xArgs = append(xArgs, sp...)
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	}

	argsLen := len(args)
	var ttyScanner *bufio.Scanner
	if p.prompt {
		ttyPath := p.tty
		if ttyPath == "" {
			ttyPath = defaultTTY
		}
		f, err := os.Open(ttyPath)
		if err != nil {
			return err
		}
		defer f.Close()
		ttyScanner = bufio.NewScanner(f)
	}

	for i := 0; i < len(xArgs); i += p.maxArgs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		m := min(i+p.maxArgs, len(xArgs))
		args = append(args, xArgs[i:m]...)

		if p.prompt {
			fmt.Fprintf(ioEnv.Stderr, "%s...?", strings.Join(args, " "))
		} else if p.trace {
			fmt.Fprintf(ioEnv.Stderr, "%s\n", strings.Join(args, " "))
		}

		if p.prompt {
			if !ttyScanner.Scan() {
				break
			}
			input := ttyScanner.Text()
			if !strings.HasPrefix(input, "y") && !strings.HasPrefix(input, "Y") {
				args = args[:argsLen]
				continue
			}
		}

		if err := executeCmd(ctx, ioEnv, args); err != nil {
			return err
		}

		args = args[:argsLen]
	}

	return nil
}

// Main runs the xargs command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "xargs: panic: %v\n", r)
			exitCode = 1
		}
	}()

	var maxArgs int
	var trace, prompt, null bool
	fs := flag.NewFlagSet("xargs", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.IntVar(&maxArgs, "n", defaultMaxArgs, "max number of arguments per command")
	fs.BoolVar(&trace, "t", false, "enable trace mode, each command is written to stderr")
	fs.BoolVar(&prompt, "p", false, "the user is asked whether to execute utility at each invocation")
	fs.BoolVar(&null, "0", false, "use a null byte as the input argument delimiter")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "xargs: %v\n", err)
		}
		return 1
	}

	p := params{
		maxArgs: maxArgs,
		trace:   trace || prompt,
		prompt:  prompt,
		null:    null,
	}

	if err := run(ctx, ioEnv, p, fs.Args()...); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "xargs: %v\n", err)
		return 1
	}
	return 0
}
