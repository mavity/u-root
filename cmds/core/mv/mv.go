// Copyright 2012-2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// mv renames files and directories.
//
// Synopsis:
//
//	mv SOURCE [-u] TARGET
//	mv SOURCE... [-u] DIRECTORY
//
// Author:
//
//	Beletti (rhiguita@gmail.com)
package mv

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type flags struct {
	update    bool
	noClobber bool
}

var errUsage = errors.New("insufficient arguments")

func moveFile(source string, dest string, update bool, noClobber bool) error {
	if noClobber {
		_, err := os.Lstat(dest)
		if !errors.Is(err, os.ErrNotExist) {
			// This is either a real error if something unexpected happen during Lstat or nil
			return err
		}
	}

	if update {
		sourceInfo, err := os.Lstat(source)
		if err != nil {
			return err
		}

		destInfo, err := os.Lstat(dest)
		if errors.Is(err, os.ErrNotExist) {
			return os.Rename(source, dest)
		}
		if err != nil {
			return err
		}
		// Check if the destination already exists and was touched later than the source
		if destInfo.ModTime().After(sourceInfo.ModTime()) {
			// Source is older and we don't want to "downgrade"
			return nil
		}
	}

	return os.Rename(source, dest)
}

func mvWithCtx(ctx context.Context, files []string, update, noClobber, todir bool) error {
	if len(files) == 2 && !todir {
		// Rename/move a single file
		if err := moveFile(files[0], files[1], update, noClobber); err != nil {
			return err
		}
	} else {
		// Move one or more files into a directory
		destdir := files[len(files)-1]
		for _, f := range files[:len(files)-1] {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			newPath := filepath.Join(destdir, filepath.Base(f))
			if err := moveFile(f, newPath, update, noClobber); err != nil {
				return err
			}
		}
	}
	return nil
}

func mv(files []string, update, noClobber, todir bool) error {
	return mvWithCtx(context.Background(), files, update, noClobber, todir)
}

func moveWithCtx(ctx context.Context, stderr io.Writer, args []string, resolvePath func(string) string) error {
	var f flags

	fs := flag.NewFlagSet("mv", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.BoolVar(&f.update, "u", false, "move only when the SOURCE file is newer than the destination file or when the destination file is missing")
	fs.BoolVar(&f.noClobber, "n", false, "do not overwrite an existing file")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: mv [ARGS] source target [ARGS] source ... directory\n\n")
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

	files := fs.Args()

	var todir bool
	dest := files[len(files)-1]
	resolvedDest := dest
	if resolvePath != nil {
		resolvedDest = resolvePath(dest)
	}

	if destdir, err := os.Lstat(resolvedDest); err == nil {
		todir = destdir.IsDir()
	}

	if len(files) > 2 && !todir {
		return fmt.Errorf("%s: %w", dest, syscall.ENOTDIR)
	}

	resolvedFiles := make([]string, len(files))
	for i, file := range files {
		if resolvePath != nil {
			resolvedFiles[i] = resolvePath(file)
		} else {
			resolvedFiles[i] = file
		}
	}

	return mvWithCtx(ctx, resolvedFiles, f.update, f.noClobber, todir)
}

func move(stderr io.Writer, args []string) error {
	return moveWithCtx(context.Background(), stderr, args, nil)
}

// Main runs mv reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "mv panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := moveWithCtx(ctx, ioEnv.Stderr, args, ioEnv.ResolvePath); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "mv: %v\n", err)
		return 1
	}
	return 0
}
