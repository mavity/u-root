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
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type flags struct {
	update    bool
	noClobber bool
	overwrite overwriteMode
}

type overwriteMode uint8

const (
	overwriteDefault overwriteMode = iota
	overwriteForce
	overwriteInteractive
	overwriteNoClobber
)

type overwriteFlag struct {
	flags *flags
	mode  overwriteMode
}

func (f overwriteFlag) String() string {
	return strconv.FormatBool(f.flags.overwrite == f.mode)
}

func (f overwriteFlag) IsBoolFlag() bool { return true }

func (f overwriteFlag) Set(value string) error {
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if enabled {
		f.flags.overwrite = f.mode
	} else if f.flags.overwrite == f.mode {
		f.flags.overwrite = overwriteDefault
	}
	f.flags.noClobber = f.flags.overwrite == overwriteNoClobber
	return nil
}

var errUsage = errors.New("insufficient arguments")

func moveFile(source string, dest string, update bool, noClobber bool) error {
	mode := overwriteDefault
	if noClobber {
		mode = overwriteNoClobber
	}
	return moveFileWithPolicy(source, dest, update, mode, nil, io.Discard, nil)
}

func moveFileWithPolicy(source string, dest string, update bool, overwrite overwriteMode, prompt *bufio.Reader, stderr io.Writer, ctx context.Context) error {
	destInfo, destErr := os.Lstat(dest)
	destExists := destErr == nil
	if destErr != nil && !errors.Is(destErr, os.ErrNotExist) {
		return destErr
	}
	if destExists && overwrite == overwriteNoClobber {
		return nil
	}
	if update {
		sourceInfo, err := os.Lstat(source)
		if err != nil {
			return err
		}
		if destExists && !sourceInfo.ModTime().After(destInfo.ModTime()) {
			// Source is older and we don't want to "downgrade"
			return nil
		}
	}

	if destExists && overwrite == overwriteInteractive {
		if prompt == nil {
			return errors.New("cannot prompt for overwrite: stdin is unavailable")
		}
		if _, err := fmt.Fprintf(stderr, "mv: overwrite %q? ", dest); err != nil {
			return fmt.Errorf("write overwrite prompt: %w", err)
		}
		answer, err := prompt.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read overwrite confirmation: %w", err)
		}
		if strings.TrimSpace(answer) == "" && errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y") {
			return nil
		}
	}

	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	if err := os.Rename(source, dest); err != nil {
		if overwrite != overwriteForce || !destExists || destInfo.IsDir() || !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := os.Remove(dest); err != nil {
			return err
		}
		return os.Rename(source, dest)
	}
	return nil
}

func mvWithCtx(ctx context.Context, files []string, update, noClobber, todir bool) error {
	mode := overwriteDefault
	if noClobber {
		mode = overwriteNoClobber
	}
	return mvWithPolicyCtx(ctx, files, update, mode, todir, nil, io.Discard)
}

func mvWithPolicyCtx(ctx context.Context, files []string, update bool, overwrite overwriteMode, todir bool, prompt *bufio.Reader, stderr io.Writer) error {
	if len(files) == 2 && !todir {
		// Rename/move a single file
		if err := moveFileWithPolicy(files[0], files[1], update, overwrite, prompt, stderr, ctx); err != nil {
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
			if err := moveFileWithPolicy(f, newPath, update, overwrite, prompt, stderr, ctx); err != nil {
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
	return moveWithIO(ctx, nil, stderr, args, resolvePath)
}

func moveWithIO(ctx context.Context, stdin io.Reader, stderr io.Writer, args []string, resolvePath func(string) string) error {
	var f flags

	fs := flag.NewFlagSet("mv", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.BoolVar(&f.update, "u", false, "move only when the SOURCE file is newer than the destination file or when the destination file is missing")
	fs.Var(overwriteFlag{flags: &f, mode: overwriteForce}, "f", "do not prompt before overwriting")
	fs.Var(overwriteFlag{flags: &f, mode: overwriteForce}, "force", "do not prompt before overwriting")
	fs.Var(overwriteFlag{flags: &f, mode: overwriteInteractive}, "i", "prompt before overwrite")
	fs.Var(overwriteFlag{flags: &f, mode: overwriteInteractive}, "interactive", "prompt before overwrite")
	fs.Var(overwriteFlag{flags: &f, mode: overwriteNoClobber}, "n", "do not overwrite an existing file")
	fs.Var(overwriteFlag{flags: &f, mode: overwriteNoClobber}, "no-clobber", "do not overwrite an existing file")

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

	var prompt *bufio.Reader
	if f.overwrite == overwriteInteractive {
		if stdin == nil {
			return errors.New("cannot prompt for overwrite: stdin is unavailable")
		}
		prompt = bufio.NewReader(stdin)
	}
	return mvWithPolicyCtx(ctx, resolvedFiles, f.update, f.overwrite, todir, prompt, stderr)
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

	if err := moveWithIO(ctx, ioEnv.Stdin, ioEnv.Stderr, args, ioEnv.ResolvePath); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "mv: %v\n", err)
		return 1
	}
	return 0
}
