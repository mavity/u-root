// Copyright 2018-2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stringscmd finds printable strings.
package stringscmd

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

var (
	ErrInvalidFormatArgument = fmt.Errorf("invalid argument to option -t")
	ErrInvalidMinLength      = fmt.Errorf("invalid minimum string length -n")

	errInvalidFormatArgument = ErrInvalidFormatArgument
	errInvalidMinLength      = ErrInvalidMinLength
)

type cmd struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	params
	args   []string
	offset int
}

type params struct {
	t string
	n int
}

func command(stdin io.Reader, stdout io.Writer, p params, args []string) *cmd {
	return &cmd{
		stdin:  stdin,
		stdout: stdout,
		params: p,
		args:   args,
	}
}

func (c *cmd) run() error {
	ioEnv := core.IO{
		Stdin:  c.stdin,
		Stdout: c.stdout,
		Stderr: c.stderr,
	}
	return runStrings(context.Background(), ioEnv, c.params, c.args)
}

func asciiIsPrint(char byte) bool {
	return char >= 32 && char <= 126
}

func offsetValue(t string, offset int) string {
	switch t {
	case "d":
		return fmt.Sprintf("%d ", offset)
	case "o":
		return fmt.Sprintf("%o ", offset)
	case "x":
		return fmt.Sprintf("%x ", offset)
	default:
		return ""
	}
}

func stringsIO(ctx context.Context, r *bufio.Reader, w io.Writer, p params) error {
	var o []byte
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		b, err := r.ReadByte()
		if err == io.EOF {
			if len(o) >= p.n {
				if p.t != "" {
					_, _ = w.Write([]byte(offsetValue(p.t, offset-len(o))))
				}
				_, _ = w.Write(o)
				_, _ = w.Write([]byte{'\n'})
			}
			return nil
		}
		if err != nil {
			return err
		}
		if !asciiIsPrint(b) {
			if len(o) >= p.n {
				if p.t != "" {
					_, _ = w.Write([]byte(offsetValue(p.t, offset-len(o))))
				}
				_, _ = w.Write(o)
				_, _ = w.Write([]byte{'\n'})
			}
			o = o[:0]
			offset++
			continue
		}
		if len(o) >= p.n+1024 {
			_, _ = w.Write(o[:1024])
			o = o[1024:]
		}
		o = append(o, b)
		offset++
	}
}

func runStrings(ctx context.Context, ioEnv core.IO, p params, args []string) error {
	if p.n < 1 {
		return fmt.Errorf("%w: %d", errInvalidMinLength, p.n)
	}
	if p.t != "" && p.t != "d" && p.t != "o" && p.t != "x" {
		return fmt.Errorf("%w: %s", errInvalidFormatArgument, p.t)
	}
	if len(args) == 0 {
		rb := bufio.NewReader(ioEnv.Stdin)
		return stringsIO(ctx, rb, ioEnv.Stdout, p)
	}
	for _, file := range args {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if file == "-" {
			rb := bufio.NewReader(ioEnv.Stdin)
			if err := stringsIO(ctx, rb, ioEnv.Stdout, p); err != nil {
				return err
			}
		} else {
			resolved := ioEnv.ResolvePath(file)
			f, err := os.Open(resolved)
			if err != nil {
				return err
			}
			rb := bufio.NewReader(f)
			err = stringsIO(ctx, rb, ioEnv.Stdout, p)
			_ = f.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Main runs the strings command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "strings: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var p params
	fs := flag.NewFlagSet("strings", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.IntVar(&p.n, "n", 4, "the minimum string length")
	fs.StringVar(&p.t, "t", "", "write each string preceded by its byte offset from the start of the file (d decimal, o octal, x hexadecimal)")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runStrings(ctx, ioEnv, p, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "strings: %v\n", err)
		return 1
	}
	return 0
}
