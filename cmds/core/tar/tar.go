// Copyright 2019 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Create and extract tar archives.
//
// Synopsis:
//
//	tar [OPTION...] [FILE]...
//
// Description:
//
//	This command line can be used only in the following ways:
//	   tar -cvf x.tar directory/         # create
//	   tar -cvf x.tar file1 file2 ...    # create
//	   tar -tvf x.tar                    # list
//	   tar -xvf x.tar directory/         # extract
//
// Options:
//
//	-c: create a new tar archive from the given directory
//	-x: extract a tar archive to the given directory
//	-v: verbose, print each filename (optional)
//	-f: tar filename (required)
//	-t: list the contents of an archive
package tarcmd

import (
	"archive/tar"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/tarutil"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type params struct {
	file        string
	create      bool
	extract     bool
	list        bool
	noRecursion bool
	verbose     bool
}

var (
	errCreateAndExtract     = fmt.Errorf("cannot supply both -c and -x")
	errCreateAndList        = fmt.Errorf("cannot supply both -c and -t")
	errExtractAndList       = fmt.Errorf("cannot supply both -x and -t")
	errEmptyFile            = fmt.Errorf("file is required")
	errMissingMandatoryFlag = fmt.Errorf("must supply at least one of: -c, -x, -t")
	errExtractArgsLen       = fmt.Errorf("args length should be 1")
)

func validate(p params, args []string) error {
	if p.create && p.extract {
		return errCreateAndExtract
	}
	if p.create && p.list {
		return errCreateAndList
	}
	if p.extract && p.list {
		return errExtractAndList
	}
	if p.extract && len(args) != 1 {
		return errExtractArgsLen
	}
	if !p.extract && !p.create && !p.list {
		return errMissingMandatoryFlag
	}
	if p.file == "" {
		return errEmptyFile
	}
	return nil
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var p params
	f := flag.NewFlagSet("tar", flag.ContinueOnError)
	f.SetOutput(ioEnv.Stderr)

	f.BoolVar(&p.create, "create", false, "create a new tar archive from the given directory")
	f.BoolVar(&p.create, "c", false, "create a new tar archive from the given directory (shorthand)")

	f.BoolVar(&p.extract, "extract", false, "extract a tar archive from the given directory")
	f.BoolVar(&p.extract, "x", false, "extract a tar archive from the given directory (shorthand)")

	f.StringVar(&p.file, "file", "", "tar file")
	f.StringVar(&p.file, "f", "", "tar file (shorthand)")

	f.BoolVar(&p.list, "list", false, "list the contents of an archive")
	f.BoolVar(&p.list, "t", false, "list the contents of an archive (shorthand)")

	f.BoolVar(&p.noRecursion, "no-recursion", false, "do not automatically recurse into directories")

	f.BoolVar(&p.verbose, "verbose", false, "print each filename")
	f.BoolVar(&p.verbose, "v", false, "print each filename (shorthand)")

	f.Usage = func() {
		fmt.Fprintf(f.Output(), "Usage of tar:\n")
		f.PrintDefaults()
	}

	if err := f.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	rest := f.Args()
	if err := validate(p, rest); err != nil {
		f.Usage()
		return err
	}

	opts := &tarutil.Opts{
		NoRecursion:     p.noRecursion,
		ChangeDirectory: ioEnv.Dir,
	}
	if p.verbose {
		opts.Filters = []tarutil.Filter{func(hdr *tar.Header) bool {
			fmt.Fprintln(ioEnv.Stdout, hdr.Name)
			return true
		}}
	}

	filePath := ioEnv.ResolvePath(p.file)

	switch {
	case p.create:
		tarFile, err := os.Create(filePath)
		if err != nil {
			return err
		}
		if err := tarutil.CreateTar(tarFile, rest, opts); err != nil {
			tarFile.Close()
			return err
		}
		if err := tarFile.Close(); err != nil {
			return err
		}

	case p.extract:
		tarFile, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer tarFile.Close()

		extractDir := rest[0]
		if err := tarutil.ExtractDir(tarFile, extractDir, opts); err != nil {
			return err
		}

	case p.list:
		tarFile, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer tarFile.Close()

		tr := tar.NewReader(tarFile)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			fmt.Fprintln(ioEnv.Stdout, hdr.Name)
		}
	}

	return nil
}

// Main runs the tar command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "tar: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "tar: %v\n", err)
		}
		return 1
	}
	return 0
}
