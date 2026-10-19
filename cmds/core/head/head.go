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

var errCombine = fmt.Errorf("can't combine line and byte counts")

func run(ctx context.Context, ioEnv core.IO, bytes, count int, files ...string) error {
	if bytes > 0 && count > 0 {
		return errCombine
	}

	var printBytes bool
	var buffer []byte
	if bytes > 0 {
		printBytes = true
		buffer = make([]byte, 4096)
	}

	if count == 0 {
		count = 10
	}

	var newLineHeader bool
	var errs error

	handle := func(r io.Reader, name string) error {
		if len(files) > 1 {
			if newLineHeader {
				fmt.Fprintf(ioEnv.Stdout, "\n==> %s <==\n", name)
			} else {
				fmt.Fprintf(ioEnv.Stdout, "==> %s <==\n", name)
				newLineHeader = true
			}
		}
		if printBytes {
			c := bytes
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				n, err := io.ReadFull(r, buffer)
				if err == io.EOF {
					break
				}
				if err != nil && err != io.ErrUnexpectedEOF {
					return err
				}

				_, _ = ioEnv.Stdout.Write(buffer[:min(c, n)])
				c -= n
				if c <= 0 {
					break
				}

				// handle the case when user request more bytes than
				// source have
				if err == io.ErrUnexpectedEOF {
					break
				}
			}
		} else {
			var c int
			scanner := bufio.NewScanner(r)
			for scanner.Scan() {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				fmt.Fprintln(ioEnv.Stdout, scanner.Text())
				c++
				if c == count {
					break
				}
			}
			if err := scanner.Err(); err != nil {
				return err
			}
		}
		return nil
	}

	// handle stdin
	if len(files) == 0 {
		return handle(ioEnv.Stdin, "")
	}

	for _, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resolved := ioEnv.ResolvePath(file)
		f, err := os.Open(resolved)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("head: %w", err))
			continue
		}
		err = handle(f, file)
		_ = f.Close()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	if errs != nil {
		fmt.Fprintf(ioEnv.Stderr, "%v\n", errs)
		return errs
	}
	return nil
}

// Main runs the head command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "head: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("head", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	c := fs.Int("c", 0, "Print bytes of each of the specified files")
	n := fs.Int("n", 0, "Print count lines of each of the specified files")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := run(ctx, ioEnv, *c, *n, fs.Args()...); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		return 1
	}
	return 0
}
