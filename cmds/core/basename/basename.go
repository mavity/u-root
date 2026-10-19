// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Basename return name with leading path information removed.
//
// Synopsis:
//
//	basename NAME [SUFFIX]
//	basename OPTION... NAME...
package basename

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errUsage = errors.New("usage: basename NAME [SUFFIX] or basename OPTION... NAME...")

func printOne(stdout interface{ Write([]byte) (int, error) }, name, suffix string, zero bool) error {
	base := filepath.Base(name)
	if suffix != "" && base != suffix {
		base = strings.TrimSuffix(base, suffix)
	}
	delimiter := "\n"
	if zero {
		delimiter = "\x00"
	}
	_, err := fmt.Fprintf(stdout, "%s%s", base, delimiter)
	return err
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var (
		multiple bool
		suffix   string
		zero     bool
	)

	fs := flag.NewFlagSet("basename", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&multiple, "a", false, "support multiple arguments and treat each as a NAME")
	fs.BoolVar(&multiple, "multiple", false, "support multiple arguments and treat each as a NAME")
	fs.StringVar(&suffix, "s", "", "remove a trailing SUFFIX; implies -a")
	fs.StringVar(&suffix, "suffix", "", "remove a trailing SUFFIX; implies -a")
	fs.BoolVar(&zero, "z", false, "end each output line with NUL, not newline")
	fs.BoolVar(&zero, "zero", false, "end each output line with NUL, not newline")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: basename NAME [SUFFIX]\n       basename OPTION... NAME...\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	rest := fs.Args()
	if suffix != "" {
		multiple = true
	}

	if multiple {
		if len(rest) == 0 {
			return errUsage
		}
		for _, name := range rest {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err := printOne(ioEnv.Stdout, name, suffix, zero); err != nil {
				return err
			}
		}
		return nil
	}

	switch len(rest) {
	case 2:
		return printOne(ioEnv.Stdout, rest[0], rest[1], zero)
	case 1:
		return printOne(ioEnv.Stdout, rest[0], "", zero)
	default:
		return errUsage
	}
}

// Main runs the basename command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "basename: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "basename: %v\n", err)
		}
		return 1
	}
	return 0
}
