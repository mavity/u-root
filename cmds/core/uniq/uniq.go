// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package uniq removes repeated lines.
package uniq

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func shouldPrint(unique, duplicates bool, cnt int) bool {
	if unique && cnt > 1 || duplicates && cnt == 1 {
		return false
	}
	return true
}

func printLine(w io.Writer, line []byte, count bool, cnt int) {
	if count {
		_, _ = fmt.Fprintf(w, "%d\t%s\n", cnt, line)
	} else {
		_, _ = fmt.Fprintf(w, "%s\n", line)
	}
}

func uniqReader(ctx context.Context, r io.Reader, w io.Writer, unique, duplicates, count bool, equal func(a, b []byte) bool) error {
	bs := bufio.NewScanner(r)

	if !bs.Scan() {
		return bs.Err()
	}
	prevLine := bytes.Clone(bs.Bytes())
	cnt := 1

	for bs.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := bytes.Clone(bs.Bytes())
		if equal(line, prevLine) {
			cnt++
		} else {
			if shouldPrint(unique, duplicates, cnt) {
				printLine(w, prevLine, count, cnt)
			}
			cnt = 1
			prevLine = line
		}
	}
	if err := bs.Err(); err != nil {
		return err
	}

	if shouldPrint(unique, duplicates, cnt) {
		printLine(w, prevLine, count, cnt)
	}
	return nil
}

func runUniq(ctx context.Context, ioEnv core.IO, unique, duplicates, count, ignoreCase bool, args []string) error {
	eq := bytes.Equal
	if ignoreCase {
		eq = bytes.EqualFold
	}
	if len(args) == 0 {
		return uniqReader(ctx, ioEnv.Stdin, ioEnv.Stdout, unique, duplicates, count, eq)
	}
	for _, fn := range args {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if fn == "-" {
			if err := uniqReader(ctx, ioEnv.Stdin, ioEnv.Stdout, unique, duplicates, count, eq); err != nil {
				return err
			}
		} else {
			resolved := ioEnv.ResolvePath(fn)
			f, err := os.Open(resolved)
			if err != nil {
				return err
			}
			err = uniqReader(ctx, f, ioEnv.Stdout, unique, duplicates, count, eq)
			_ = f.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// run provides backward compatibility for unit tests.
func run(stdin io.Reader, stdout io.Writer, unique, duplicates, count, ignoreCase bool, args []string) error {
	ioEnv := core.IO{
		Stdin:  stdin,
		Stdout: stdout,
	}
	return runUniq(context.Background(), ioEnv, unique, duplicates, count, ignoreCase, args)
}

// Main runs the uniq command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "uniq: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var (
		unique     bool
		duplicates bool
		count      bool
		ignoreCase bool
	)
	fs := flag.NewFlagSet("uniq", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&unique, "u", false, "print unique lines")
	fs.BoolVar(&duplicates, "d", false, "print one copy of duplicated lines")
	fs.BoolVar(&count, "c", false, "prefix a repetition count and a tab for each output line")
	fs.BoolVar(&ignoreCase, "i", false, "case insensitive comparison of lines")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runUniq(ctx, ioEnv, unique, duplicates, count, ignoreCase, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "uniq: %v\n", err)
		return 1
	}
	return 0
}
