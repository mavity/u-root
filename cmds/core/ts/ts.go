// Copyright 2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ts prepends each line of stdin with a timestamp.
package ts

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/u-root/u-root/pkg/core"
	pkgts "github.com/u-root/u-root/pkg/ts"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func runTS(ctx context.Context, ioEnv core.IO, first, relative bool, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("invalid use")
	}

	t := pkgts.New(ioEnv.Stdin)
	t.ResetTimeOnNextRead = first
	if relative {
		t.Format = pkgts.NewRelativeFormat()
	}

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		nr, er := t.Read(buf)
		if nr > 0 {
			nw, ew := ioEnv.Stdout.Write(buf[:nr])
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

// run provides backward compatibility for unit tests.
func run(stdin io.Reader, stdout io.Writer, args []string) error {
	var first, relative bool
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.BoolVar(&first, "f", false, "All timestamps are relative to the first character")
	fs.BoolVar(&relative, "R", false, "Timestamps are relative to the previous timestamp")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args[1:])); err != nil {
		return err
	}

	ioEnv := core.IO{
		Stdin:  stdin,
		Stdout: stdout,
	}
	return runTS(context.Background(), ioEnv, first, relative, fs.Args())
}

// Main runs the ts command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "ts: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var first, relative bool
	fs := flag.NewFlagSet("ts", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&first, "f", false, "All timestamps are relative to the first character")
	fs.BoolVar(&relative, "R", false, "Timestamps are relative to the previous timestamp")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runTS(ctx, ioEnv, first, relative, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "ts: %v\n", err)
		return 1
	}
	return 0
}
