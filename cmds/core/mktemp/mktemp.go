// Copyright 2012-2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Mktemp makes a temporary file (or directory)
//
// Synopsis:
//
//	mktemp [OPTION]... [TEMPLATE]
//
// Options:
//
//	-d, --directory: create a directory, not a file
//	-u, --dry-run: do not create anything; merely print a name (unsafe)
//	-q, --quiet: suppress diagnostics about file/dir-creation failure
//	--suffix=SUFF: append SUFF to TEMPLATE
//	-p DIR, --tmpdir[=DIR]: interpret TEMPLATE relative to DIR
package mktemp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type flags struct {
	d      bool
	u      bool
	q      bool
	t      bool
	prefix string
	suffix string
	dir    string
}

type quietError struct {
	err error
}

func (e quietError) Error() string {
	return e.err.Error()
}

func (e quietError) Unwrap() error {
	return e.err
}

func (f *flags) register(fs *flag.FlagSet) {
	fs.BoolVar(&f.d, "directory", false, "Make a directory")
	fs.BoolVar(&f.d, "d", false, "Make a directory (shorthand)")

	fs.BoolVar(&f.u, "dry-run", false, "Do everything save the actual create")
	fs.BoolVar(&f.u, "u", false, "Do everything save the actual create (shorthand)")

	fs.BoolVar(&f.q, "quiet", false, "Quiet: show no errors")
	fs.BoolVar(&f.q, "q", false, "Quiet: show no errors (shorthand)")

	fs.BoolVar(&f.t, "t", false, "Generate a template relative to TMPDIR")

	fs.StringVar(&f.prefix, "prefix", "", "add a prefix")
	fs.StringVar(&f.prefix, "s", "", "add a prefix (shorthand, 's' is for compatibility with GNU mktemp)")

	fs.StringVar(&f.suffix, "suffix", "", "add a suffix to the prefix (rather than the end of the mktemp file)")

	fs.StringVar(&f.dir, "tmpdir", "", "Tmp directory to use. If this is not set, TMPDIR is used, else default temp dir")
	fs.StringVar(&f.dir, "p", "", "Tmp directory to use. If this is not set, TMPDIR is used, else default temp dir (shorthand)")
}

func mktemp(ioEnv core.IO, f flags) (string, error) {
	dir := f.dir
	if dir == "" {
		if tmpdir := ioEnv.Getenv("TMPDIR"); tmpdir != "" {
			dir = tmpdir
		} else {
			dir = os.TempDir()
		}
	} else {
		dir = ioEnv.ResolvePath(dir)
	}

	if f.u {
		if !f.q {
			fmt.Fprintf(ioEnv.Stderr, "mktemp: warning: dry-run is inherently unsafe\n")
		}
		prefix := f.prefix
		if prefix == "" {
			prefix = "tmp"
		}
		return filepath.Join(dir, prefix+strconv.FormatUint(rand.Uint64(), 10)), nil
	}

	if f.d {
		return os.MkdirTemp(dir, f.prefix)
	}
	file, err := os.CreateTemp(dir, f.prefix)
	if err != nil {
		return "", err
	}
	_ = file.Close()
	return file.Name(), nil
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var f flags
	fs := flag.NewFlagSet("mktemp", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	f.register(fs)

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: mktemp [options] [template]\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	if f.t && f.dir == "" {
		f.dir = ioEnv.Getenv("TMPDIR")
		if f.dir == "" {
			f.dir = os.TempDir()
		}
	}

	switch fs.NArg() {
	case 1:
		f.prefix = f.prefix + strings.Split(fs.Args()[0], "X")[0] + f.suffix
	case 0:
	default:
		fs.Usage()
		return fmt.Errorf("too many arguments")
	}

	fileName, err := mktemp(ioEnv, f)
	if err != nil {
		if f.q {
			return quietError{err: err}
		}
		return err
	}

	fmt.Fprintf(ioEnv.Stdout, "%s\n", fileName)
	return nil
}

// Main runs the mktemp command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "mktemp: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		var quietErr quietError
		if !errors.Is(err, flag.ErrHelp) && !errors.As(err, &quietErr) {
			fmt.Fprintf(ioEnv.Stderr, "mktemp: %v\n", err)
		}
		return 1
	}
	return 0
}
