// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cmp compares two files and prints a message if their contents differ.
package cmp

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rck/unit"
	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

const (
	usage = "usage:[-options] file1 file 2 [offset1 [offset 2]]"
)

var (
	ErrArgCount  = errors.New("arg count")
	ErrBadOffset = errors.New("bad offset")
	ErrDiffer    = errors.New("files differ")
)

func readFileOrStdin(ioEnv core.IO, name string) (io.Reader, io.Closer, int64, error) {
	if name == "-" {
		return ioEnv.Stdin, nil, 0, nil
	}
	resolved := ioEnv.ResolvePath(name)
	f, err := os.Open(resolved)
	if err != nil {
		return nil, nil, 0, err
	}
	return f, f, 0, nil
}

func runCmp(ctx context.Context, ioEnv core.IO, long, line, silent bool, args ...string) error {
	var offset [2]int64
	cmpUnits := unit.DefaultUnits

	off, err := unit.NewUnit(cmpUnits)
	if err != nil {
		return fmt.Errorf("could not create unit based on mapping: %w", err)
	}

	var v *unit.Value
	switch len(args) {
	case 2:
	case 3:
		if v, err = off.ValueFromString(args[2]); err != nil {
			if ioEnv.Stderr != nil {
				fmt.Fprintf(ioEnv.Stderr, "bad offset1: %s: %v", args[2], err)
			}
			return fmt.Errorf("%w:%w", err, ErrBadOffset)
		}
		offset[0] = v.Value
	case 4:
		if v, err = off.ValueFromString(args[2]); err != nil {
			if ioEnv.Stderr != nil {
				fmt.Fprintf(ioEnv.Stderr, "bad offset1: %s: %v", args[2], err)
			}
			return fmt.Errorf("%w:%w", err, ErrBadOffset)
		}
		offset[0] = v.Value

		if v, err = off.ValueFromString(args[3]); err != nil {
			if ioEnv.Stderr != nil {
				fmt.Fprintf(ioEnv.Stderr, "bad offset2: %s: %v", args[3], err)
			}
			return fmt.Errorf("%w:%w", err, ErrBadOffset)
		}
		offset[1] = v.Value
	default:
		if ioEnv.Stderr != nil {
			fmt.Fprint(ioEnv.Stderr, usage)
		}
		return ErrArgCount
	}

	readers := make([]io.Reader, 2)
	for i := 0; i < 2; i++ {
		r, cl, _, err := readFileOrStdin(ioEnv, args[i])
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", args[i], err)
		}
		if cl != nil {
			defer cl.Close()
		}
		if seeker, ok := r.(io.Seeker); ok {
			if _, err := seeker.Seek(offset[i], io.SeekStart); err != nil {
				return fmt.Errorf("%w:%w", err, ErrBadOffset)
			}
		} else if offset[i] > 0 {
			if _, err := io.CopyN(io.Discard, r, offset[i]); err != nil {
				return fmt.Errorf("%w:%w", err, ErrBadOffset)
			}
		}
		readers[i] = bufio.NewReader(r)
	}

	lineno, charno := int64(1), int64(1)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var b [2]byte
		_, err1 := readers[0].Read(b[:1])
		_, err2 := readers[1].Read(b[1:2])

		if err1 != nil || err2 != nil {
			if err1 == io.EOF && err2 == io.EOF {
				return nil
			}
			if err1 != nil {
				if ioEnv.Stderr != nil {
					fmt.Fprintf(ioEnv.Stderr, "%s:%v", args[0], err1)
				}
				return err1
			}
			if err2 != nil {
				if ioEnv.Stderr != nil {
					fmt.Fprintf(ioEnv.Stderr, "%s:%v", args[1], err2)
				}
				return err2
			}
		}

		b1, b2 := b[0], b[1]
		if b1 != b2 {
			if silent {
				return ErrDiffer
			}
			if line {
				if ioEnv.Stdout != nil {
					fmt.Fprintf(ioEnv.Stdout, "%s %s: char %d line %d", args[0], args[1], charno, lineno)
				}
				return ErrDiffer
			}
			if long {
				if ioEnv.Stdout != nil {
					fmt.Fprintf(ioEnv.Stdout, "%8d %#.2o %#.2o\n", charno, b1, b2)
				}
			} else {
				if ioEnv.Stdout != nil {
					fmt.Fprintf(ioEnv.Stdout, "%s %s: char %d", args[0], args[1], charno)
				}
				return ErrDiffer
			}
		}
		charno++
		if b1 == '\n' {
			lineno++
		}
	}
}

// cmp provides backward compatibility for unit tests.
func cmp(stdout, stderr io.Writer, long, line, silent bool, args ...string) error {
	ioEnv := core.IO{
		Stdin:  os.Stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	err := runCmp(context.Background(), ioEnv, long, line, silent, args...)
	if silent && errors.Is(err, ErrDiffer) {
		return nil
	}
	return err
}

// Main runs the cmp command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "cmp: internal panic: %v\n", r)
			exitCode = 2
		}
	}()

	var (
		long   bool
		line   bool
		silent bool
	)
	fs := flag.NewFlagSet("cmp", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&long, "l", false, "print the byte number (decimal) and the differing bytes (hexadecimal) for each difference")
	fs.BoolVar(&line, "L", false, "print the line number of the first differing byte")
	fs.BoolVar(&silent, "s", false, "print nothing for differing files, but set the exit status")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	err := runCmp(ctx, ioEnv, long, line, silent, fs.Args()...)
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrDiffer) {
		return 1
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 130
	}
	return 2
}
