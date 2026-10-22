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
	"unicode/utf8"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func shouldPrint(unique, duplicates bool, cnt int) bool {
	if unique && cnt > 1 || duplicates && cnt == 1 {
		return false
	}
	return true
}

func printLine(w io.Writer, line []byte, count bool, cnt int) error {
	if count {
		_, err := fmt.Fprintf(w, "%d\t%s\n", cnt, line)
		return err
	}
	_, err := fmt.Fprintf(w, "%s\n", line)
	return err
}

func comparisonKey(line []byte, skipFields, skipChars int) []byte {
	pos := 0
	for field := 0; field < skipFields; field++ {
		for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
			pos++
		}
		for pos < len(line) && line[pos] != ' ' && line[pos] != '\t' {
			pos++
		}
	}
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}
	for chars := 0; chars < skipChars && pos < len(line); chars++ {
		_, size := utf8.DecodeRune(line[pos:])
		pos += size
	}
	return line[pos:]
}

func uniqReader(ctx context.Context, r io.Reader, w io.Writer, unique, duplicates, count bool, equal func(a, b []byte) bool, skipFields, skipChars int) error {
	reader := bufio.NewReader(r)
	line, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return err
	}
	if len(line) == 0 {
		return nil
	}
	if line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	prevLine := bytes.Clone(line)
	prevKey := bytes.Clone(comparisonKey(prevLine, skipFields, skipChars))
	cnt := 1

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err = reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return err
		}
		if len(line) == 0 {
			break
		}
		if line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		line = bytes.Clone(line)
		key := comparisonKey(line, skipFields, skipChars)
		if equal(key, prevKey) {
			cnt++
		} else {
			if shouldPrint(unique, duplicates, cnt) {
				if err := printLine(w, prevLine, count, cnt); err != nil {
					return err
				}
			}
			cnt = 1
			prevLine = line
			prevKey = bytes.Clone(key)
		}
	}

	if shouldPrint(unique, duplicates, cnt) {
		return printLine(w, prevLine, count, cnt)
	}
	return nil
}

func runUniq(ctx context.Context, ioEnv core.IO, unique, duplicates, count, ignoreCase bool, skipFields, skipChars int, args []string) error {
	eq := bytes.Equal
	if ignoreCase {
		eq = bytes.EqualFold
	}
	if len(args) == 0 {
		return uniqReader(ctx, ioEnv.Stdin, ioEnv.Stdout, unique, duplicates, count, eq, skipFields, skipChars)
	}
	for _, fn := range args {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if fn == "-" {
			if err := uniqReader(ctx, ioEnv.Stdin, ioEnv.Stdout, unique, duplicates, count, eq, skipFields, skipChars); err != nil {
				return err
			}
		} else {
			resolved := ioEnv.ResolvePath(fn)
			f, err := os.Open(resolved)
			if err != nil {
				return err
			}
			err = uniqReader(ctx, f, ioEnv.Stdout, unique, duplicates, count, eq, skipFields, skipChars)
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
	return runUniq(context.Background(), ioEnv, unique, duplicates, count, ignoreCase, 0, 0, args)
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
		skipFields int
		skipChars  int
	)
	fs := flag.NewFlagSet("uniq", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&unique, "u", false, "print unique lines")
	fs.BoolVar(&duplicates, "d", false, "print one copy of duplicated lines")
	fs.BoolVar(&count, "c", false, "prefix a repetition count and a tab for each output line")
	fs.BoolVar(&ignoreCase, "i", false, "case insensitive comparison of lines")
	fs.IntVar(&skipFields, "f", 0, "avoid comparing the first N fields")
	fs.IntVar(&skipChars, "s", 0, "avoid comparing the first N characters")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if skipFields < 0 || skipChars < 0 {
		fmt.Fprintln(ioEnv.Stderr, "uniq: skip counts must not be negative")
		return 2
	}
	if err := runUniq(ctx, ioEnv, unique, duplicates, count, ignoreCase, skipFields, skipChars, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "uniq: %v\n", err)
		return 1
	}
	return 0
}
