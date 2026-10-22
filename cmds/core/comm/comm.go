// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package comm compares two files.
package comm

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

// ErrUsage is the error for incorrect usage.
var ErrUsage = errors.New("comm: comm [-123h] file1 file2")

func readLines(ctx context.Context, r io.Reader) ([]string, error) {
	reader := bufio.NewReader(r)
	var lines []string
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if strings.HasSuffix(line, "\n") {
				line = strings.TrimSuffix(line, "\n")
				line = strings.TrimSuffix(line, "\r")
			}
			lines = append(lines, line)
		}
		if err != nil {
			if err == io.EOF {
				return lines, nil
			}
			return nil, err
		}
	}
}

func openStream(ioEnv core.IO, arg string) (io.Reader, io.Closer, error) {
	if arg == "-" {
		return ioEnv.Stdin, nil, nil
	}
	resolved := ioEnv.ResolvePath(arg)
	f, err := os.Open(resolved)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

func runComm(ctx context.Context, ioEnv core.IO, s1, s2, s3, help bool, args ...string) error {
	if len(args) != 2 || help {
		return ErrUsage
	}

	r1, cl1, err := openStream(ioEnv, args[0])
	if err != nil {
		return err
	}
	if cl1 != nil {
		defer cl1.Close()
	}
	r2, cl2, err := openStream(ioEnv, args[1])
	if err != nil {
		return err
	}
	if cl2 != nil {
		defer cl2.Close()
	}

	lines1, err := readLines(ctx, r1)
	if err != nil {
		return err
	}
	lines2, err := readLines(ctx, r2)
	if err != nil {
		return err
	}

	i, j := 0, 0
	for i < len(lines1) || j < len(lines2) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var col1, col2, col3 string
		if i < len(lines1) && j < len(lines2) {
			switch {
			case lines1[i] < lines2[j]:
				col1 = lines1[i]
				i++
			case lines1[i] > lines2[j]:
				col2 = lines2[j]
				j++
			default:
				col3 = lines1[i]
				i++
				j++
			}
		} else if i < len(lines1) {
			col1 = lines1[i]
			i++
		} else {
			col2 = lines2[j]
			j++
		}

		line := ""
		if !s1 {
			line += col1
		}
		line += "\t"
		if !s2 {
			line += col2
		}
		line += "\t"
		if !s3 {
			line += col3
		}
		if line != "\t\t" {
			if _, err := fmt.Fprintln(ioEnv.Stdout, strings.TrimRight(line, "\t")); err != nil {
				return err
			}
		}
	}
	return nil
}

// comm provides backward compatibility for unit tests.
func comm(w io.Writer, s1, s2, s3, help bool, args ...string) error {
	ioEnv := core.IO{
		Stdout: w,
	}
	return runComm(context.Background(), ioEnv, s1, s2, s3, help, args...)
}

// Main runs the comm command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "comm: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var s1, s2, s3, help bool
	fs := flag.NewFlagSet("comm", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&s1, "1", false, "suppress printing of column 1")
	fs.BoolVar(&s2, "2", false, "suppress printing of column 2")
	fs.BoolVar(&s3, "3", false, "suppress printing of column 3")
	fs.BoolVar(&help, "h", false, "print this help message and exit")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runComm(ctx, ioEnv, s1, s2, s3, help, fs.Args()...); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "comm: %v\n", err)
		return 1
	}
	return 0
}
