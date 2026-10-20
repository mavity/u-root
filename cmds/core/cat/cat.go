// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// cat concatenates files and prints them to stdout.
//
// Synopsis:
//
//	cat [-u] [FILES]...
//
// Description:
//
//	If no files are specified, read from stdin.
//
// Options:
//
//	-u: ignored flag (unbuffered in standard Unix cat)
package cat

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errCopy = fmt.Errorf("error concatenating to stdout")

func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "pipe is being closed") ||
		strings.Contains(msg, "i/o error")
}

func catStream(ctx context.Context, r io.Reader, w io.Writer) error {
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				if isBrokenPipe(werr) {
					return nil
				}
				return fmt.Errorf("%w: %w", errCopy, werr)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("%w: %w", errCopy, err)
		}
	}
}

// Main executes the cat command with the given execution context.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "cat: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var u bool
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&u, "u", false, "ignored unbuffered flag")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: cat [-u] [FILES]...\n\ncat concatenates files and prints them to stdout.\nIf no files are specified, read from stdin.\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	select {
	case <-ctx.Done():
		return 130
	default:
	}

	files := fs.Args()
	if len(files) == 0 {
		if err := catStream(ctx, ioEnv.Stdin, ioEnv.Stdout); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return 130
			}
			fmt.Fprintf(ioEnv.Stderr, "cat: %v\n", err)
			return 1
		}
		return 0
	}

	for _, file := range files {
		select {
		case <-ctx.Done():
			return 130
		default:
		}

		if file == "-" {
			if err := catStream(ctx, ioEnv.Stdin, ioEnv.Stdout); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return 130
				}
				fmt.Fprintf(ioEnv.Stderr, "cat: %v\n", err)
				return 1
			}
			continue
		}

		resolvedPath := ioEnv.ResolvePath(file)
		f, err := os.Open(resolvedPath)
		if err != nil {
			fmt.Fprintf(ioEnv.Stderr, "cat: %v\n", err)
			exitCode = 1
			continue
		}

		err = catStream(ctx, f, ioEnv.Stdout)
		_ = f.Close()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return 130
			}
			fmt.Fprintf(ioEnv.Stderr, "cat: %v\n", err)
			return 1
		}
	}

	return exitCode
}
