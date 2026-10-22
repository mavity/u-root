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
	"strconv"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type offsetWriter struct {
	writer io.Writer
	offset uint64
	line   []byte
}

func (w *offsetWriter) writeLine() error {
	line := w.line
	if len(line) >= 8 {
		if base, err := strconv.ParseUint(string(line[:8]), 16, 64); err == nil {
			prefix := []byte(fmt.Sprintf("%08x", base+w.offset))
			line = append(prefix, line[8:]...)
		}
	}
	line = append(line, '\n')
	n, err := w.writer.Write(line)
	if err != nil {
		return err
	}
	if n != len(line) {
		return io.ErrShortWrite
	}
	w.line = w.line[:0]
	return nil
}

func (w *offsetWriter) Write(p []byte) (int, error) {
	for i, b := range p {
		if b == '\n' {
			if err := w.writeLine(); err != nil {
				return i + 1, err
			}
			continue
		}
		w.line = append(w.line, b)
	}
	return len(p), nil
}

func (w *offsetWriter) Close() error {
	if len(w.line) == 0 {
		return nil
	}
	return w.writeLine()
}

func runHexdump(ctx context.Context, ioEnv core.IO, filenames []string, maxBytes, skip int64) (retErr error) {
	var readers []io.Reader
	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			if err := c.Close(); retErr == nil && err != nil {
				retErr = err
			}
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
	if skip > 0 {
		_, err := io.CopyN(io.Discard, r, skip)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
	if maxBytes >= 0 {
		r = io.LimitReader(r, maxBytes)
	}

	adjusted := &offsetWriter{writer: ioEnv.Stdout, offset: uint64(skip)}
	w := hex.Dumper(adjusted)
	defer func() {
		if err := w.Close(); retErr == nil && err != nil {
			retErr = err
		}
		if err := adjusted.Close(); retErr == nil && err != nil {
			retErr = err
		}
	}()

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
	return runHexdump(context.Background(), ioEnv, filenames, -1, 0)
}

// Main runs the hexdump command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "hexdump: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var (
		canonical bool
		verbose   bool
		maxBytes  int64
		skip      int64
	)
	maxBytes = -1
	fs := flag.NewFlagSet("hexdump", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&canonical, "C", false, "canonical hex+ASCII display")
	fs.BoolVar(&verbose, "v", false, "do not suppress repeated output lines")
	fs.Int64Var(&maxBytes, "n", -1, "interpret only N bytes of input")
	fs.Int64Var(&skip, "s", 0, "skip N bytes from the beginning")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if maxBytes < -1 || skip < 0 {
		fmt.Fprintln(ioEnv.Stderr, "hexdump: byte counts must not be negative")
		return 2
	}
	_ = canonical
	_ = verbose
	if err := runHexdump(ctx, ioEnv, fs.Args(), maxBytes, skip); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "hexdump: %v\n", err)
		return 1
	}
	return 0
}
