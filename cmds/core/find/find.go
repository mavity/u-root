// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Find finds files. It is similar to the Unix command. It uses REs, not globs,
// for matching.
//
// OPTIONS:
//
//	-d: enable debugging in the find package
//	-mode integer-arg: match against mode, e.g. -mode 0755
//	-type: match against a file type, e.g. -type f will match files
//	-name: glob to match against file name
//	-regex: regular expression to match against file name
//	-l: long listing. It's not very good, yet, but it's useful enough.
package findcmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/find"
)

var (
	errNotValidType = errors.New("not a valid file type")
	errUsage        = errors.New("find [opts] starting-at-path")
)

type flags struct {
	fileType string
	name     string
	regex    string
	perm     int
	long     bool
	debug    bool
}

type cmd struct {
	flags
	stdout io.Writer
	stderr io.Writer
	args   []string
	opts   []find.Set
}

// reorderArgs reorders arguments so flags are moved to the front, which is the
// way the "flag" package can parse them.
func reorderArgs(args []string) []string {
	var (
		newArgs []string
		i       int
	)

	for i < len(args) {
		var (
			arg          = args[i]
			expectsValue = arg == "-name" || arg == "-regex" || arg == "-type" || arg == "-mode"
			hasNext      = i+1 < len(args)
			isFlag       = strings.HasPrefix(arg, "-")
		)

		prepend := func(args ...string) {
			newArgs = append(args, newArgs...)
		}

		switch {
		case expectsValue && hasNext:
			next := args[i+1]
			prepend(arg, next)
			i += 2
		case isFlag:
			prepend(arg)
			i++
		default:
			newArgs = append(newArgs, arg)
			i++
		}
	}

	return newArgs
}

func command(ioEnv core.IO, args ...string) (*cmd, error) {
	var f flags

	fs := flag.NewFlagSet("find", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	fs.StringVar(&f.fileType, "type", "", "file type")
	fs.StringVar(&f.name, "name", "", "glob for name")
	fs.StringVar(&f.regex, "regex", "", "regex for name")
	fs.IntVar(&f.perm, "mode", -1, "permissions")
	fs.BoolVar(&f.long, "l", false, "long listing")
	fs.BoolVar(&f.debug, "d", false, "enable debugging in the find package")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: find [opts] starting-at-path\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgs(args)); err != nil {
		return nil, err
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return nil, errUsage
	}

	c := &cmd{
		stdout: ioEnv.Stdout,
		stderr: ioEnv.Stderr,
		args:   fs.Args(),
		flags:  f,
	}

	fileTypes := map[string]os.FileMode{
		"f":         0,
		"file":      0,
		"d":         os.ModeDir,
		"directory": os.ModeDir,
		"s":         os.ModeSocket,
		"p":         os.ModeNamedPipe,
		"l":         os.ModeSymlink,
		"c":         os.ModeCharDevice | os.ModeDevice,
		"b":         os.ModeDevice,
	}

	root := ioEnv.ResolvePath(c.args[0])

	c.opts = []find.Set{
		find.WithRoot(root),
	}

	var mask, mode os.FileMode
	var modemaskmatch bool
	if c.perm != -1 {
		mask = os.ModePerm
		mode = os.FileMode(c.perm)
		modemaskmatch = true
	}
	if c.fileType != "" {
		intType, ok := fileTypes[c.fileType]
		if !ok {
			var keys []string
			for key := range fileTypes {
				keys = append(keys, key)
			}
			return nil, fmt.Errorf("%w: %v\n valid types are %v", errNotValidType, c.fileType, strings.Join(keys, ","))
		}
		mode |= intType
		mask |= os.ModeType
		modemaskmatch = true
	}

	if modemaskmatch {
		c.opts = append(c.opts, find.WithModeMatch(mode, mask))
	}

	if c.debug {
		c.opts = append(c.opts, find.WithDebugLog(log.Printf))
	}

	if len(c.name) > 0 {
		c.opts = append(c.opts, find.WithFilenameMatch(c.name))
	}

	if len(c.regex) > 0 {
		o, err := find.WithCompiledRegexPathMatch(c.regex)
		if err != nil {
			return nil, err
		}
		c.opts = append(c.opts, o)
	}

	return c, nil
}

func (c *cmd) run(ctx context.Context, ioEnv core.IO) error {
	names := find.Find(ctx, c.opts...)

	rootPrefix := ioEnv.ResolvePath(c.args[0])
	origRoot := c.args[0]

	for l := range names {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if l.Err != nil {
			fmt.Fprintf(c.stderr, "%s: %v\n", l.Name, l.Err)
			continue
		}

		displayName := l.Name
		if rootPrefix != origRoot && strings.HasPrefix(l.Name, rootPrefix) {
			rel := strings.TrimPrefix(l.Name, rootPrefix)
			rel = strings.TrimPrefix(rel, string(os.PathSeparator))
			if rel == "" {
				displayName = origRoot
			} else if origRoot == "." {
				displayName = rel
			} else {
				displayName = origRoot + string(os.PathSeparator) + rel
			}
		}

		if c.long {
			fmt.Fprintf(c.stdout, "%s\n", l)
			continue
		}
		fmt.Fprintf(c.stdout, "%s\n", displayName)
	}

	return nil
}

// Main runs the find command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "find: panic: %v\n", r)
			exitCode = 1
		}
	}()

	c, err := command(ioEnv, args...)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "find: %v\n", err)
		}
		return 1
	}

	if err := c.run(ctx, ioEnv); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "find: %v\n", err)
		return 1
	}
	return 0
}
