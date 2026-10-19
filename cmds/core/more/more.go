// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// More pages through files without any terminal trickery.
//
// Synopsis:
//
//	more [OPTIONS] FILE...
//
// Description:
//
//	Admittedly, this does not follow the conventions of GNU more. Instead,
//	it is built with the goal of not relying on any special ttys, ioctls or
//	special ANSI escapes. This is ideal when your terminal is already
//	borked. For bells and whistles, look at less.
//
// Options:
//
//	--lines NUMBER: screen size in number of lines
package more

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errLinesMustBePositive = fmt.Errorf("lines must be positive")

func pageStream(ctx context.Context, stdin io.Reader, stdout io.Writer, r io.Reader, lines int, interactive bool) error {
	scanner := bufio.NewScanner(r)
	for i := 0; scanner.Scan(); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if interactive && (i+1)%lines == 0 {
			if stdout != nil {
				fmt.Fprint(stdout, scanner.Text())
			}
			if stdin != nil {
				c := make([]byte, 1)
				// We expect the OS to echo the newline character.
				if _, err := stdin.Read(c); err != nil {
					if errors.Is(err, io.EOF) {
						if stdout != nil {
							fmt.Fprintln(stdout)
						}
						break
					}
					return err
				}
			}
		} else {
			if stdout != nil {
				fmt.Fprintln(stdout, scanner.Text())
			}
		}
	}
	return scanner.Err()
}

func runWithCtx(ctx context.Context, stdin io.Reader, stdout io.Writer, lines int, resolvePath func(string) string, args []string) error {
	if lines <= 0 {
		return fmt.Errorf("%d: %w", lines, errLinesMustBePositive)
	}

	if len(args) == 0 {
		return pageStream(ctx, stdin, stdout, stdin, lines, false)
	}

	for _, arg := range args {
		if arg == "-" {
			if err := pageStream(ctx, stdin, stdout, stdin, lines, false); err != nil {
				return err
			}
			continue
		}
		p := arg
		if resolvePath != nil {
			p = resolvePath(p)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		err = pageStream(ctx, stdin, stdout, f, lines, true)
		_ = f.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func run(stdin io.Reader, stdout io.Writer, lines int, args []string) error {
	return runWithCtx(context.Background(), stdin, stdout, lines, nil, args)
}

// Main runs the more command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "more panic: %v\n", r)
			exitCode = 1
		}
	}()

	var lines int
	fs := flag.NewFlagSet("more", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.IntVar(&lines, "lines", 40, "screen size in number of lines")
	fs.IntVar(&lines, "n", 40, "screen size in number of lines")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if err := runWithCtx(ctx, ioEnv.Stdin, ioEnv.Stdout, lines, ioEnv.ResolvePath, fs.Args()); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "more: %v\n", err)
		return 1
	}
	return 0
}
