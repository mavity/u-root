// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// ls prints the contents of a directory.
//
// Synopsis:
//
//	ls [OPTIONS] [DIRS]...
//
// Options:
//
//	-a: show hidden files
//	-h: show human-readable sizes
//	-d: show directories but not their contents
//	-F: append indicator (, one of */=>@|) to entries
//	-l: long form
//	-Q: quoted
//	-R: equivalent to findutil's find
//	-S: sort by size
//	-p: print final path element (Plan 9)
package lscmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/ls"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type cmd struct {
	w         io.Writer
	all       bool
	human     bool
	directory bool
	long      bool
	quoted    bool
	recurse   bool
	classify  bool
	size      bool
	final     bool
}

type file struct {
	path string
	osfi os.FileInfo
	lsfi ls.FileInfo
	err  error
}

func (c cmd) listNameWithCtx(ctx context.Context, stringer ls.Stringer, d string, prefix bool, resolvePath func(string) string) (bool, error) {
	var files []file
	hasError := false

	resolved := d
	if resolvePath != nil {
		resolved = resolvePath(d)
	}

	walkErr := filepath.Walk(resolved, func(path string, osfi os.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		f := file{
			path: path,
			osfi: osfi,
		}

		// error handling that matches standard ls is ... a real joy
		if osfi != nil && !errors.Is(err, os.ErrNotExist) {
			f.lsfi = ls.FromOSFileInfo(path, osfi)
			if err != nil && path == resolved {
				f.err = err
			}
		} else {
			f.err = err
		}

		files = append(files, f)

		if err != nil {
			return filepath.SkipDir
		}

		if !c.recurse && path == resolved && c.directory {
			return filepath.SkipDir
		}

		if path != resolved && f.lsfi.Mode.IsDir() && !c.recurse {
			return filepath.SkipDir
		}

		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, filepath.SkipDir) {
		return true, walkErr
	}

	if c.size {
		sort.SliceStable(files, func(i, j int) bool {
			return files[i].lsfi.Size > files[j].lsfi.Size
		})
	}

	for _, f := range files {
		if f.err != nil {
			hasError = true
			c.printFile(stringer, f)
			continue
		}
		if c.recurse {
			// Mimic find command
			if d != resolved {
				if rel, err := filepath.Rel(resolved, f.path); err == nil && rel != "." {
					f.lsfi.Name = filepath.Join(d, rel)
				} else {
					f.lsfi.Name = d
				}
			} else {
				f.lsfi.Name = f.path
			}
		} else if f.path == resolved {
			if c.directory {
				fmt.Fprintln(c.w, stringer.FileString(f.lsfi))
				continue
			}

			// Starting directory is a dot when non-recursive
			if f.osfi != nil && f.osfi.IsDir() {
				f.lsfi.Name = "."
				if prefix {
					if c.quoted {
						fmt.Fprintf(c.w, "%q:\n", d)
					} else {
						fmt.Fprintf(c.w, "%v:\n", d)
					}
				}
			}
		}

		c.printFile(stringer, f)
	}

	return hasError, nil
}

func (c cmd) listName(stringer ls.Stringer, d string, prefix bool) error {
	_, err := c.listNameWithCtx(context.Background(), stringer, d, prefix, nil)
	return err
}

func indicator(fi ls.FileInfo) string {
	if fi.Mode.IsRegular() && fi.Mode&0o111 != 0 {
		return "*"
	}
	if fi.Mode&os.ModeDir != 0 {
		return "/"
	}
	if fi.Mode&os.ModeSymlink != 0 {
		return "@"
	}
	if fi.Mode&os.ModeSocket != 0 {
		return "="
	}
	if fi.Mode&os.ModeNamedPipe != 0 {
		return "|"
	}
	return ""
}

func (c cmd) list(ctx context.Context, names []string, resolvePath func(string) string) (bool, error) {
	if len(names) == 0 {
		names = []string{"."}
	}
	// Write output in tabular form.
	tw := &tabwriter.Writer{}
	tw.Init(c.w, 0, 0, 1, ' ', 0)
	c.w = tw
	defer tw.Flush()

	var s ls.Stringer = ls.NameStringer{}
	if c.quoted {
		s = ls.QuotedStringer{}
	}
	if c.long {
		s = ls.LongStringer{Human: c.human, Name: s}
	}
	// Is a name a directory? If so, list it in its own section.
	prefix := len(names) > 1
	hasAnyError := false
	for _, d := range names {
		hadErr, err := c.listNameWithCtx(ctx, s, d, prefix, resolvePath)
		if err != nil {
			return true, fmt.Errorf("error while listing %q: %w", d, err)
		}
		if hadErr {
			hasAnyError = true
		}
		tw.Flush()
	}
	return hasAnyError, nil
}

// run runs a command. args are as from os.Args, i.e., args[0] is the command name.
func run(w io.Writer, args []string) error {
	var c cmd
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.BoolVar(&c.all, "a", false, "show hidden files")
	f.BoolVar(&c.human, "h", false, "human readable sizes")
	f.BoolVar(&c.directory, "d", false, "list directories but not their contents")
	f.BoolVar(&c.long, "l", false, "long form")
	f.BoolVar(&c.quoted, "Q", false, "quoted")
	f.BoolVar(&c.recurse, "R", false, "equivalent to findutil's find")
	f.BoolVar(&c.classify, "F", false, "append indicator (, one of */=>@|) to entries")
	f.BoolVar(&c.size, "S", false, "sort by size")
	f.BoolVar(&c.final, "p", false, "Print only the final path element of each file name")
	c.w = w

	f.Usage = func() {
		fmt.Fprintf(f.Output(), "Usage: ls [OPTIONS] [DIRS]...\n\n")
		fmt.Fprintf(f.Output(), "Options:\n")
		f.PrintDefaults()
	}

	if err := f.Parse(unixflag.ArgsToGoArgs(args[1:])); err != nil {
		return err
	}
	_, err := c.list(context.Background(), f.Args(), nil)
	return err
}

// Main runs ls reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "ls panic: %v\n", r)
			exitCode = 1
		}
	}()

	var c cmd
	f := flag.NewFlagSet("ls", flag.ContinueOnError)
	f.SetOutput(ioEnv.Stderr)
	f.BoolVar(&c.all, "a", false, "show hidden files")
	f.BoolVar(&c.human, "h", false, "human readable sizes")
	f.BoolVar(&c.directory, "d", false, "list directories but not their contents")
	f.BoolVar(&c.long, "l", false, "long form")
	f.BoolVar(&c.quoted, "Q", false, "quoted")
	f.BoolVar(&c.recurse, "R", false, "equivalent to findutil's find")
	f.BoolVar(&c.classify, "F", false, "append indicator (, one of */=>@|) to entries")
	f.BoolVar(&c.size, "S", false, "sort by size")
	f.BoolVar(&c.final, "p", false, "Print only the final path element of each file name")
	c.w = ioEnv.Stdout

	if err := f.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	hasError, err := c.list(ctx, f.Args(), ioEnv.ResolvePath)
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "ls: %v\n", err)
		return 1
	}
	if hasError {
		return 1
	}
	return 0
}
