// Copyright 2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// base64 - encode and decode base64 from stdin or file to stdout
//
// Synopsis:
//
//	base64 [-d] [FILE]
//
// Description:
//
//	Encode or decode a file to or from base64 encoding.
//	-d   decode data (default is to encode)
//	For stdin, on standard Unix systems, you can use /dev/stdin
package base64cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type cmd struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	params
	args []string
}

type params struct {
	decode bool
}

var errBadUsage = errors.New("usage: base64 [-d] [file]")

type cancelReader struct {
	ctx context.Context
	r   io.Reader
}

func (c cancelReader) Read(p []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	default:
		return c.r.Read(p)
	}
}

func decodeone(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	r := base64.NewDecoder(base64.StdEncoding, cancelReader{ctx: ctx, r: stdin})
	if _, err := io.Copy(stdout, r); err != nil {
		return fmt.Errorf("decoding: %w", err)
	}
	return nil
}

func encodeone(ctx context.Context, stdin io.Reader, stdout io.Writer) (retErr error) {
	// WriteCloser is important here, from NewEncoder documentation:
	// when finished writing, the caller must Close the returned encoder
	// to flush any partially written blocks.
	wc := base64.NewEncoder(base64.StdEncoding, stdout)
	closed := false
	defer func() {
		if !closed {
			if err := wc.Close(); retErr == nil && err != nil {
				retErr = fmt.Errorf("closing encoder: %w", err)
			}
		}
	}()
	if _, err := io.Copy(wc, cancelReader{ctx: ctx, r: stdin}); err != nil {
		return fmt.Errorf("error encoding: %w", err)
	}
	err := wc.Close()
	closed = true
	if err != nil {
		return fmt.Errorf("closing encoder: %w", err)
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return fmt.Errorf("error encoder writing trailing newline %w", err)
	}
	return nil
}

func runWithCtx(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, resolvePath func(string) string, args ...string) (retErr error) {
	c := cmd{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}

	f := flag.NewFlagSet("base64", flag.ContinueOnError)
	f.SetOutput(stderr)
	f.BoolVar(&c.decode, "d", false, "decode or encode the file")

	if err := f.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	c.args = f.Args()

	switch {
	case len(c.args) > 1:
		return fmt.Errorf("only 0 or 1 arg allowed: %w", errBadUsage)
	case len(c.args) == 0:
	case c.args[0] == "-":
	default:
		p := c.args[0]
		if resolvePath != nil {
			p = resolvePath(p)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer func() {
			if err := f.Close(); retErr == nil && err != nil {
				retErr = err
			}
		}()
		c.stdin = f
	}

	if c.stdin == nil {
		c.stdin = bytes.NewReader(nil)
	}

	if c.decode {
		return decodeone(ctx, c.stdin, c.stdout)
	}
	return encodeone(ctx, c.stdin, c.stdout)
}

func run(stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	return runWithCtx(context.Background(), stdin, stdout, stderr, nil, args...)
}

// Main executes base64 reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "base64 panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := runWithCtx(ctx, ioEnv.Stdin, ioEnv.Stdout, ioEnv.Stderr, ioEnv.ResolvePath, args...); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "base64: %v\n", err)
		return 1
	}
	return 0
}
