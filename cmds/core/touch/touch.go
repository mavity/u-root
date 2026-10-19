// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// touch changes file access and modification times.
//
// Synopsis:
//
//	touch [-amc] [-d datetime] file...
//
// Description:
//
//	If a file does not exist, it will be created unless -c is specified.
//
// Options:
//
//	-a: change only the access time
//	-m: change only the modification time
//	-c: do not create any file if it does not exist
//	-d: use specified time instead of current time (RFC3339 format)
package touch

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type params struct {
	time         time.Time
	access       bool
	modification bool
	create       bool
}

type cmd struct {
	params
	args []string
}

var errNoFiles = errors.New("no files specified")

func parseParams(dateTime string, access, modification, create bool) (params, error) {
	t := time.Now()
	if dateTime != "" {
		var err error
		t, err = time.Parse(time.RFC3339, dateTime)
		if err != nil {
			return params{}, err
		}
	}
	return params{
		access:       access || (!access && !modification),
		modification: modification || (!access && !modification),
		create:       create,
		time:         t,
	}, nil
}

func parseCommand(ioEnv core.IO, args ...string) (*cmd, error) {
	var c cmd
	var b bytes.Buffer
	fs := flag.NewFlagSet("touch", flag.ContinueOnError)
	fs.SetOutput(&b)

	var access bool
	var modification bool
	var create bool
	var dateTime string

	fs.BoolVar(&access, "a", false, "change only the access time")
	fs.BoolVar(&modification, "m", false, "change only the modification time")
	fs.BoolVar(&create, "c", false, "do not create any file if it does not exist")
	fs.StringVar(&dateTime, "d", "", "use specified time instead of current time RFC3339")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: touch [-amc] [-d datetime] file...\n\n")
		fmt.Fprintf(fs.Output(), "touch changes file access and modification times.\n")
		fmt.Fprintf(fs.Output(), "If a file does not exist, it will be created unless -c is specified.\n\n")
		fmt.Fprintf(fs.Output(), "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return nil, err
	}

	if len(fs.Args()) == 0 {
		fs.Usage()
		return nil, fmt.Errorf("%s:%w", b.String(), errNoFiles)
	}
	c.args = fs.Args()

	var err error
	c.params, err = parseParams(dateTime, access, modification, create)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *cmd) run(ctx context.Context, ioEnv core.IO) error {
	var errs error
	for _, arg := range c.args {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resolved := ioEnv.ResolvePath(arg)
		_, existsErr := os.Stat(resolved)
		notExist := os.IsNotExist(existsErr)
		if notExist {
			if c.create {
				continue
			}

			f, err := os.Create(resolved)
			if err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			f.Close()
		}

		accessTime := time.Time{}
		if c.access || notExist {
			accessTime = c.time
		}
		modificationTime := time.Time{}
		if c.modification || notExist {
			modificationTime = c.time
		}

		err := os.Chtimes(resolved, accessTime, modificationTime)
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

// Main runs the touch command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "touch: panic: %v\n", r)
			exitCode = 1
		}
	}()

	c, err := parseCommand(ioEnv, args...)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "touch: %v\n", err)
		}
		return 1
	}

	if err := c.run(ctx, ioEnv); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "touch: %v\n", err)
		return 1
	}
	return 0
}
