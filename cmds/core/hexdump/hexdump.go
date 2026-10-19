// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hexdump prints file content in hexadecimal.
package hexdump

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func runHexdump(ctx context.Context, ioEnv core.IO, filenames []string) error {
	var readers []io.Reader
	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()

	if len(filenames) == 0 {
		readers = []io.Reader{ioEnv.Stdin}
	} else {
		readers = make([]io.Reader, 0, len(filenames))
		for _, filename := range filenames {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if filename == "-" {
				readers = append(readers, ioEnv.Stdin)
			} else {
				resolved := ioEnv.ResolvePath(filename)
				f, err := os.Open(resolved)
				if err != nil {
					return err
				}
				closers = append(closers, f)
				readers = append(readers, f)
			}
		}
	}

	r := io.MultiReader(readers...)
	w := hex.Dumper(ioEnv.Stdout)
	defer w.Close()

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		nr, er := r.Read(buf)
		if nr > 0 {
			nw, ew := w.Write(buf[:nr])
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

// hexdump provides backward compatibility for unit tests.
func hexdump(filenames []string, reader io.Reader, writer io.Writer) error {
	ioEnv := core.IO{
		Stdin:  reader,
		Stdout: writer,
	}
	return runHexdump(context.Background(), ioEnv, filenames)
}

// Main runs the hexdump command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "hexdump: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var canonical bool
	fs := flag.NewFlagSet("hexdump", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&canonical, "C", false, "canonical hex+ASCII display")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runHexdump(ctx, ioEnv, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "hexdump: %v\n", err)
		return 1
	}
	return 0
}
