// Copyright 2012-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// mkdir makes one or more new directories.
//
// Synopsis:
//
//	mkdir [-m mode] [-v] [-p] DIRECTORY...
//
// Options:
//
//	-m: directory mode (ex: 755)
//	-v: print each directory as it is made
//	-p: make all needed directories in the path
package mkdir

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

const (
	defaultCreationMode = 0o777
	stickyBit           = 0o1000
	sgidBit             = 0o2000
	suidBit             = 0o4000
)

var errNoDirs = errors.New("no directories specified")
var errInvalidMode = errors.New("invalid mode")

type flags struct {
	mode    string
	mkall   bool
	verbose bool
}

func mkdir(ctx context.Context, ioEnv core.IO, args ...string) error {
	var f flags

	fs := flag.NewFlagSet("mkdir", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	fs.StringVar(&f.mode, "m", "", "Directory mode")
	fs.BoolVar(&f.mkall, "p", false, "Make all needed directories in the path")
	fs.BoolVar(&f.verbose, "v", false, "Print each directory as it is made")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: mkdir [-m mode] [-v] [-p] DIRECTORY...\n\n")
		fmt.Fprintf(fs.Output(), "mkdir makes one or more new directories.\n\n")
		fmt.Fprintf(fs.Output(), "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	if len(fs.Args()) < 1 {
		fs.Usage()
		return errNoDirs
	}

	fm := os.Mkdir
	if f.mkall {
		fm = os.MkdirAll
	}

	// Get Correct Creation Mode
	var m uint64
	var err error
	if f.mode == "" {
		m = defaultCreationMode
	} else {
		m, err = strconv.ParseUint(f.mode, 8, 32)
		if err != nil || m > 0o7777 {
			return fmt.Errorf("%w: %q", errInvalidMode, f.mode)
		}
	}
	createMode := os.FileMode(m)
	if m&stickyBit != 0 {
		createMode |= os.ModeSticky
	}
	if m&sgidBit != 0 {
		createMode |= os.ModeSetgid
	}
	if m&suidBit != 0 {
		createMode |= os.ModeSetuid
	}

	var errs error
	for _, name := range fs.Args() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resolvedName := ioEnv.ResolvePath(name)
		if err := fm(resolvedName, createMode); err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if f.verbose {
			fmt.Fprintf(ioEnv.Stdout, "%v\n", name)
		}
		if f.mode != "" {
			_ = os.Chmod(resolvedName, createMode)
		}
	}
	return errs
}

// Main runs the mkdir command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "mkdir: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := mkdir(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "mkdir: %v\n", err)
		}
		return 1
	}
	return 0
}
