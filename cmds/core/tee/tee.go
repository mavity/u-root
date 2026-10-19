// Copyright 2013-2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package tee transcribes the standard input to the standard output and makes copies in files.
package tee

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type cmd struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	args   []string
	cat    bool
	ignore bool
}

func (c *cmd) run() error {
	ioEnv := core.IO{
		Stdin:  c.stdin,
		Stdout: c.stdout,
		Stderr: c.stderr,
	}
	return runTee(context.Background(), ioEnv, c.cat, c.ignore, c.args)
}

func runTee(ctx context.Context, ioEnv core.IO, cat, ignore bool, args []string) error {
	oflags := os.O_WRONLY | os.O_CREATE
	if cat {
		oflags |= os.O_APPEND
	} else {
		oflags |= os.O_TRUNC
	}

	if ignore {
		signal.Ignore(os.Interrupt)
	}

	files := make([]*os.File, 0, len(args))
	writers := make([]io.Writer, 0, len(args)+1)
	for _, fname := range args {
		resolved := ioEnv.ResolvePath(fname)
		f, err := os.OpenFile(resolved, oflags, 0o666)
		if err != nil {
			return fmt.Errorf("error opening %s: %w", fname, err)
		}
		files = append(files, f)
		writers = append(writers, f)
	}
	writers = append(writers, ioEnv.Stdout)

	defer func() {
		for _, f := range files {
			if err := f.Close(); err != nil {
				if ioEnv.Stderr != nil {
					fmt.Fprintf(ioEnv.Stderr, "tee: error closing file %q: %v\n", f.Name(), err)
				}
			}
		}
	}()

	mw := io.MultiWriter(writers...)
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		nr, er := ioEnv.Stdin.Read(buf)
		if nr > 0 {
			nw, ew := mw.Write(buf[0:nr])
			if ew != nil {
				return ew
			}
			if nr != nw {
				return io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				break
			}
			return er
		}
	}

	return nil
}

func command(args []string) *cmd {
	c := &cmd{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.BoolVar(&c.cat, "append", false, "append the output to the files rather than rewriting them")
	f.BoolVar(&c.cat, "a", false, "append the output to the files rather than rewriting them")
	f.BoolVar(&c.ignore, "ignore-interrupts", false, "ignore the SIGINT signal")
	f.BoolVar(&c.ignore, "i", false, "ignore the SIGINT signal")

	_ = f.Parse(unixflag.ArgsToGoArgs(args[1:]))
	c.args = f.Args()

	return c
}

// Main runs the tee command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "tee: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var cat, ignore bool
	fs := flag.NewFlagSet("tee", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&cat, "append", false, "append the output to the files rather than rewriting them")
	fs.BoolVar(&cat, "a", false, "append the output to the files rather than rewriting them")
	fs.BoolVar(&ignore, "ignore-interrupts", false, "ignore the SIGINT signal")
	fs.BoolVar(&ignore, "i", false, "ignore the SIGINT signal")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runTee(ctx, ioEnv, cat, ignore, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "tee: %v\n", err)
		return 1
	}
	return 0
}
