// Copyright 2012-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// cat concatenates files and prints them to stdout.
//
// Synopsis:
//
//	cat [-u] [FILES]...
//
// Description:
//
//	If no files are specified, read from stdin.
//
// Options:
//
//	-u: ignored flag
package cat

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errCopy = fmt.Errorf("error concatenating stdin to stdout")

func cat(reader io.Reader, writer io.Writer) error {
	if _, err := io.Copy(writer, reader); err != nil {
		return errCopy
	}
	return nil
}

func run(stdin io.Reader, stdout io.Writer, args ...string) error {
	return runWithResolve(stdin, stdout, nil, args...)
}

func runWithResolve(stdin io.Reader, stdout io.Writer, resolve func(string) string, args ...string) error {
	if len(args) == 0 {
		return cat(stdin, stdout)
	}
	for _, file := range args {
		if file == "-" {
			err := cat(stdin, stdout)
			if err != nil {
				return err
			}
			continue
		}
		path := file
		if resolve != nil {
			path = resolve(file)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		if err := cat(f, stdout); err != nil {
			f.Close()
			return fmt.Errorf("failed to concatenate file %s to given writer", f.Name())
		}
		f.Close()
	}
	return nil
}

// Main executes the cat command with the given execution context.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "cat: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var u bool
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&u, "u", false, "ignored")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: cat [-u] [FILES]...\n\ncat concatenates files and prints them to stdout.\nIf no files are specified, read from stdin.\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	select {
	case <-ctx.Done():
		return 130
	default:
	}

	if err := runWithResolve(ioEnv.Stdin, ioEnv.Stdout, ioEnv.ResolvePath, fs.Args()...); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "cat: %v\n", err)
		return 1
	}
	return 0
}
