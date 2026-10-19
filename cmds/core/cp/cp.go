// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// cp copies files.
//
// Synopsis:
//
//	cp [-rRfivwP] FROM... TO
//
// Options:
//
//	-w n: number of worker goroutines
//	-R: copy file hierarchies
//	-r: alias to -R recursive mode
//	-i: prompt about overwriting file
//	-f: force overwrite files
//	-v: verbose copy mode
//	-P: don't follow symlinks
package cpcmd

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/cp"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errUsage = errors.New("usage: cp [-RrifvP] file[s] ... dest")

type flags struct {
	recursive        bool
	ask              bool
	force            bool
	verbose          bool
	noFollowSymlinks bool
}

// promptOverwrite ask if the user wants overwrite file
func promptOverwrite(dst string, out io.Writer, in *bufio.Reader) (bool, error) {
	fmt.Fprintf(out, "cp: overwrite %q? ", dst)
	answer, err := in.ReadString('\n')
	if err != nil {
		return false, err
	}

	if len(answer) == 0 || strings.ToLower(answer)[0] != 'y' {
		return false, nil
	}

	return true, nil
}

func setupPreCallback(recursive, ask, force bool, writer io.Writer, reader bufio.Reader) func(string, string, os.FileInfo) error {
	return func(src, dst string, srcfi os.FileInfo) error {
		// check if src is dir
		if !recursive && srcfi.IsDir() {
			fmt.Fprintf(writer, "cp: -r not specified, omitting directory %s\n", src)
			return cp.ErrSkip
		}

		dstfi, err := os.Stat(dst)
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(writer, "cp: %q: can't handle error %v\n", dst, err)
			return cp.ErrSkip
		} else if err != nil {
			// dst does not exist.
			return nil
		}

		// dst does exist.
		if os.SameFile(srcfi, dstfi) {
			fmt.Fprintf(writer, "cp: %q and %q are the same file\n", src, dst)
			return cp.ErrSkip
		}
		if ask && !force {
			overwrite, err := promptOverwrite(dst, writer, &reader)
			if err != nil {
				return err
			}
			if !overwrite {
				return cp.ErrSkip
			}
		}
		return nil
	}
}

func setupPostCallback(verbose bool, w io.Writer) func(src, dst string) {
	return func(src, dst string) {
		if verbose {
			fmt.Fprintf(w, "%q -> %q\n", src, dst)
		}
	}
}

// runWithCtx evaluates the flags and args and makes decisions for copyfiles
func runWithCtx(ctx context.Context, args []string, w io.Writer, i *bufio.Reader, resolvePath func(string) string) error {
	var f flags

	fs := flag.NewFlagSet("cp", flag.ContinueOnError)

	fs.BoolVar(&f.recursive, "RECURSIVE", false, "copy file hierarchies")
	fs.BoolVar(&f.recursive, "R", false, "copy file hierarchies (shorthand)")

	fs.BoolVar(&f.recursive, "recursive", false, "alias to -R recursive mode")
	fs.BoolVar(&f.recursive, "r", false, "alias to -R recursive mode (shorthand)")

	fs.BoolVar(&f.ask, "interactive", false, "prompt about overwriting file")
	fs.BoolVar(&f.ask, "i", false, "prompt about overwriting file (shorthand)")

	fs.BoolVar(&f.force, "force", false, "force overwrite files")
	fs.BoolVar(&f.force, "f", false, "force overwrite files (shorthand)")

	fs.BoolVar(&f.verbose, "verbose", false, "verbose copy mode")
	fs.BoolVar(&f.verbose, "v", false, "verbose copy mode (shorthand)")

	fs.BoolVar(&f.noFollowSymlinks, "no-dereference", false, "don't follow symlinks")
	fs.BoolVar(&f.noFollowSymlinks, "P", false, "don't follow symlinks (shorthand)")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: cp [-RrifvP] file[s] ... dest\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if fs.NArg() < 2 {
		fs.Usage()
		return errUsage
	}

	rawFrom := fs.Args()[:fs.NArg()-1]
	rawTo := fs.Args()[fs.NArg()-1]

	resolvedTo := rawTo
	if resolvePath != nil {
		resolvedTo = resolvePath(rawTo)
	}

	todir := false
	toStat, err := os.Stat(resolvedTo)
	if err == nil {
		todir = toStat.IsDir()
	}
	if fs.NArg() > 2 && !todir {
		return eNotDir
	}

	var reader bufio.Reader
	if i != nil {
		reader = *i
	} else {
		reader = *bufio.NewReader(strings.NewReader(""))
	}

	opts := cp.Options{
		NoFollowSymlinks: f.noFollowSymlinks,
		PreCallback:      setupPreCallback(f.recursive, f.ask, f.force, w, reader),
		PostCallback:     setupPostCallback(f.verbose, w),
	}

	var lastErr error
	for _, file := range rawFrom {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		src := file
		if resolvePath != nil {
			src = resolvePath(file)
		}
		dst := resolvedTo
		if todir {
			dst = filepath.Join(dst, filepath.Base(file))
		}
		if f.recursive {
			lastErr = opts.CopyTree(src, dst)
		} else {
			lastErr = opts.Copy(src, dst)
		}
		if lastErr != nil {
			break
		}
	}
	return lastErr
}

// run evaluates the flags and args and makes decisions for copyfiles
func run(args []string, w io.Writer, i *bufio.Reader) error {
	return runWithCtx(context.Background(), args, w, i, nil)
}

// Main runs cp reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "cp panic: %v\n", r)
			exitCode = 1
		}
	}()

	var reader *bufio.Reader
	if ioEnv.Stdin != nil {
		reader = bufio.NewReader(ioEnv.Stdin)
	}

	if err := runWithCtx(ctx, args, ioEnv.Stderr, reader, ioEnv.ResolvePath); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "cp: %v\n", err)
		return 1
	}
	return 0
}
