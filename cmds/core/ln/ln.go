// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Ln makes links to files.
//
// Synopsis:
//
//	ln [-svfTiLPrt] TARGET LINK
//
// Options:
//
//	-s: make symbolic links instead of hard links
//	-v: print name of each linked file
//	-f: remove destination files
//	-T: treat linkname operand as a non-dir always
//	-i: prompt if the user wants overwrite
//	-L: dereference targets if are symbolic links
//	-P: make hard links directly to symbolic links
//	-r: create symlinks relative to link location
//	-t: specify the directory to put the links
//
// Author:
//
//	Manoel Vilela <manoel_vilela@engineer.com>
package ln

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type config struct {
	symlink    bool
	verbose    bool
	force      bool
	nondir     bool
	prompt     bool
	logical    bool
	physical   bool
	relative   bool
	dirtgt     string
	targetKind string
}

// promptOverwrite ask for overwrite destination
func promptOverwrite(ioEnv core.IO, fname string) bool {
	fmt.Fprintf(ioEnv.Stderr, "ln: overwrite '%v'? ", fname)
	answer, err := bufio.NewReader(ioEnv.Stdin).ReadString('\n')
	return err == nil && len(answer) > 0 && strings.ToLower(answer)[0] == 'y'
}

// exists verify if a fname exists
func exists(fname string) bool {
	_, err := os.Lstat(fname)
	return !os.IsNotExist(err)
}

// evalArgs based of the four type of uses describe at ln
// get the targets and linknames operands
func (conf *config) evalArgs(ioEnv core.IO, args []string) (targets []string, linkName string) {
	if conf.dirtgt != "" || len(args) <= 1 {
		return args, ""
	}

	targets = args[:len(args)-1]
	lastArg := args[len(args)-1]

	resolvedLast := ioEnv.ResolvePath(lastArg)
	if lf, err := os.Stat(resolvedLast); !conf.nondir && err == nil && lf.IsDir() {
		conf.dirtgt = lastArg
	} else {
		linkName = lastArg
	}

	return targets, linkName
}

// relLink get the relative link path between a target and linkName fpath
func relLink(target, linkName string) (string, error) {
	base := filepath.Dir(linkName)
	if newTarget, err := filepath.Rel(base, target); err == nil {
		return newTarget, nil
	} else if absLink, err := filepath.Abs(linkName); err == nil {
		return filepath.Rel(absLink, target)
	}

	return "", nil
}

// inferLinkName infers the linkName if don't passed ("")
func inferLinkName(target, linkName string) string {
	if linkName == "" {
		linkName = filepath.Base(target)
	}
	return linkName
}

// dereferTarget treat symlinks according the flags -P and -L
func (conf config) dereferTarget(ioEnv core.IO, target string) (string, error) {
	if conf.logical && !conf.symlink {
		resolved := ioEnv.ResolvePath(target)
		if newTarget, err := filepath.EvalSymlinks(resolved); err != nil {
			return "", err
		} else if newTarget != resolved {
			target = newTarget
		}
	}
	return target, nil
}

// run controls link creation with flag handling and path virtualization.
func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var conf config
	fs := flag.NewFlagSet("ln", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	fs.BoolVar(&conf.symlink, "s", false, "make symbolic links instead of hard links")
	fs.BoolVar(&conf.verbose, "v", false, "print name of each linked file")
	fs.BoolVar(&conf.force, "f", false, "remove destination files")
	fs.BoolVar(&conf.nondir, "T", false, "treat linkname operand as a non-dir always")
	fs.BoolVar(&conf.nondir, "n", false, "do not dereference destination symlink to a directory")
	fs.BoolVar(&conf.prompt, "i", false, "prompt if the user wants overwrite")
	fs.BoolVar(&conf.logical, "L", false, "dereference targets if are symbolic links")
	fs.BoolVar(&conf.physical, "P", false, "make hard links directly to symbolic links")
	fs.BoolVar(&conf.relative, "r", false, "create symlinks relative to link location")
	fs.StringVar(&conf.dirtgt, "t", "", "specify the directory to put the links")
	fs.StringVar(&conf.targetKind, "target-kind", "", "symbolic-link target kind: file or directory")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}
	targetKind := uint32(0)
	switch conf.targetKind {
	case "":
	case "file":
		targetKind = 1
	case "directory":
		targetKind = 2
	default:
		return fmt.Errorf("ln: invalid target kind %q (want file or directory)", conf.targetKind)
	}
	if targetKind != 0 && !conf.symlink {
		return fmt.Errorf("ln: --target-kind requires -s")
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("ln: missing file operand")
	}

	var remove bool
	linkFunc := os.Link
	if conf.symlink {
		linkFunc = os.Symlink
	}

	targets, linkName := conf.evalArgs(ioEnv, rest)
	for _, target := range targets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// dereference symlinks
		t, err := conf.dereferTarget(ioEnv, target)
		if err != nil {
			return err
		}
		target = t

		lnName := inferLinkName(target, linkName)
		if conf.dirtgt != "" {
			lnName = filepath.Join(conf.dirtgt, lnName)
		}

		resolvedLink := ioEnv.ResolvePath(lnName)
		if exists(resolvedLink) {
			if conf.prompt && !conf.force {
				remove = promptOverwrite(ioEnv, resolvedLink)
				if !remove {
					continue
				}
			}

			if conf.force || remove {
				if err := os.Remove(resolvedLink); err != nil {
					return err
				}
			}
		}

		if conf.relative && !conf.symlink {
			return fmt.Errorf("cannot do -r without -s")
		}

		resolvedTarget := target
		if !conf.symlink {
			resolvedTarget = ioEnv.ResolvePath(target)
		}

		// make relative paths with symlinks
		if conf.relative {
			relTarget, err := relLink(ioEnv.ResolvePath(target), resolvedLink)
			if err != nil {
				return err
			}
			resolvedTarget = relTarget
		}

		if conf.symlink {
			err = createSymlink(resolvedTarget, resolvedLink, targetKind)
		} else {
			err = linkFunc(resolvedTarget, resolvedLink)
		}
		if err != nil {
			return err
		}

		if conf.verbose {
			fmt.Fprintf(ioEnv.Stdout, "%q -> %q\n", lnName, target)
		}
	}

	return nil
}

// Main runs the ln command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "ln: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "ln: %v\n", err)
		}
		return 1
	}
	return 0
}
