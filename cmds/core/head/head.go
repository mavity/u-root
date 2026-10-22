// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package head

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
)

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func copyBytes(ctx context.Context, r io.Reader, w io.Writer, count int64) error {
	buffer := make([]byte, 32*1024)
	for count > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		chunk := buffer
		if int64(len(chunk)) > count {
			chunk = chunk[:count]
		}
		n, err := r.Read(chunk)
		if n > 0 {
			if writeErr := writeAll(w, chunk[:n]); writeErr != nil {
				return writeErr
			}
			count -= int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}

func copyLines(ctx context.Context, r io.Reader, w io.Writer, count int64) error {
	reader := bufio.NewReader(r)
	for count > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if writeErr := writeAll(w, line); writeErr != nil {
				return writeErr
			}
			count--
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
	return nil
}

func run(ctx context.Context, ioEnv core.IO, count int64, byteCount bool, files ...string) error {
	if ioEnv.Stdout == nil {
		return errors.New("head: stdout is unavailable")
	}

	copyInput := copyLines
	if byteCount {
		copyInput = copyBytes
	}

	printHeader := len(files) > 1
	headerPrinted := false
	var errs error
	handle := func(name string, r io.Reader) error {
		if printHeader {
			if headerPrinted {
				if _, err := fmt.Fprintln(ioEnv.Stdout); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(ioEnv.Stdout, "==> %s <==\n", name); err != nil {
				return err
			}
			headerPrinted = true
		}
		return copyInput(ctx, r, ioEnv.Stdout, count)
	}

	for _, name := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if name == "-" {
			if err := handle(name, ioEnv.Stdin); err != nil {
				errs = errors.Join(errs, fmt.Errorf("standard input: %w", err))
			}
			continue
		}

		file, err := os.Open(ioEnv.ResolvePath(name))
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		err = handle(name, file)
		closeErr := file.Close()
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s: %w", name, err))
		}
		if closeErr != nil {
			errs = errors.Join(errs, fmt.Errorf("%s: %w", name, closeErr))
		}
	}
	if len(files) == 0 {
		if err := copyInput(ctx, ioEnv.Stdin, ioEnv.Stdout, count); err != nil {
			return fmt.Errorf("standard input: %w", err)
		}
	}
	return errs
}

// Main runs the head command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			if ioEnv.Stderr != nil {
				fmt.Fprintf(ioEnv.Stderr, "head: internal panic: %v\n", r)
			}
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("head", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	byteCount := fs.Int64("c", -1, "Print the first COUNT bytes of each file")
	lineCount := fs.Int64("n", -1, "Print the first COUNT lines of each file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	var hasByteCount, hasLineCount bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "c":
			hasByteCount = true
		case "n":
			hasLineCount = true
		}
	})
	if hasByteCount && hasLineCount {
		if ioEnv.Stderr != nil {
			fmt.Fprintln(ioEnv.Stderr, "head: cannot combine -c and -n")
		}
		return 2
	}
	count := int64(10)
	isByteCount := hasByteCount
	if hasByteCount {
		count = *byteCount
	} else if hasLineCount {
		count = *lineCount
	}
	if (hasByteCount && *byteCount < 0) || (hasLineCount && *lineCount < 0) {
		if ioEnv.Stderr != nil {
			fmt.Fprintln(ioEnv.Stderr, "head: count must not be negative")
		}
		return 2
	}

	if err := run(ctx, ioEnv, count, isByteCount, fs.Args()...); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		if ioEnv.Stderr != nil {
			fmt.Fprintf(ioEnv.Stderr, "head: %v\n", err)
		}
		return 1
	}
	return 0
}
