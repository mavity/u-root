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
	"strconv"
	"strings"
	"time"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type params struct {
	time         time.Time
	access       bool
	modification bool
	create       bool
	reference    string
}

type cmd struct {
	params
	args []string
}

var errNoFiles = errors.New("no files specified")

func parseParams(dateTime, stamp, reference string, access, modification, create bool) (params, error) {
	t := time.Now()
	if dateTime != "" && stamp != "" {
		return params{}, errors.New("-d and -t cannot be used together")
	}
	if reference != "" && (dateTime != "" || stamp != "") {
		return params{}, errors.New("-r cannot be used with -d or -t")
	}
	if dateTime != "" {
		var err error
		t, err = time.Parse(time.RFC3339, dateTime)
		if err != nil {
			return params{}, err
		}
	} else if stamp != "" {
		var err error
		t, err = parseTimestamp(stamp, t)
		if err != nil {
			return params{}, err
		}
	}
	return params{
		access:       access || (!access && !modification),
		modification: modification || (!access && !modification),
		create:       create,
		time:         t,
		reference:    reference,
	}, nil
}

func parseTimestamp(stamp string, now time.Time) (time.Time, error) {
	digits := stamp
	second := 0
	if i := strings.IndexByte(stamp, '.'); i >= 0 {
		if i != len(stamp)-3 || strings.IndexByte(stamp[i+1:], '.') >= 0 {
			return time.Time{}, fmt.Errorf("invalid timestamp %q", stamp)
		}
		parsed, err := strconv.Atoi(stamp[i+1:])
		if err != nil || parsed > 59 {
			return time.Time{}, fmt.Errorf("invalid timestamp %q", stamp)
		}
		second = parsed
		digits = stamp[:i]
	}

	if len(digits) != 8 && len(digits) != 10 && len(digits) != 12 {
		return time.Time{}, fmt.Errorf("invalid timestamp %q", stamp)
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return time.Time{}, fmt.Errorf("invalid timestamp %q", stamp)
		}
	}

	year, offset := now.Year(), 0
	switch len(digits) {
	case 10:
		year, _ = strconv.Atoi(digits[:2])
		if year < 69 {
			year += 2000
		} else {
			year += 1900
		}
		offset = 2
	case 12:
		year, _ = strconv.Atoi(digits[:4])
		offset = 4
	}
	month, _ := strconv.Atoi(digits[offset : offset+2])
	day, _ := strconv.Atoi(digits[offset+2 : offset+4])
	hour, _ := strconv.Atoi(digits[offset+4 : offset+6])
	minute, _ := strconv.Atoi(digits[offset+6 : offset+8])

	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
	if int(t.Month()) != month || t.Day() != day || t.Hour() != hour || t.Minute() != minute || t.Second() != second {
		return time.Time{}, fmt.Errorf("invalid timestamp %q", stamp)
	}
	return t, nil
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
	var stamp string
	var reference string

	fs.BoolVar(&access, "a", false, "change only the access time")
	fs.BoolVar(&modification, "m", false, "change only the modification time")
	fs.BoolVar(&create, "c", false, "do not create any file if it does not exist")
	fs.StringVar(&dateTime, "d", "", "use specified time instead of current time RFC3339")
	fs.StringVar(&stamp, "t", "", "use [[CC]YY]MMDDhhmm[.ss] instead of current time")
	fs.StringVar(&reference, "r", "", "use this file's times instead of current time")
	fs.StringVar(&reference, "reference", "", "use this file's times instead of current time")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: touch [-amc] [-d datetime | -t stamp | -r file] file...\n\n")
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
	c.params, err = parseParams(dateTime, stamp, reference, access, modification, create)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *cmd) run(ctx context.Context, ioEnv core.IO) error {
	var referenceAccess, referenceModification time.Time
	if c.reference != "" {
		referencePath := ioEnv.ResolvePath(c.reference)
		referenceInfo, err := os.Stat(referencePath)
		if err != nil {
			return fmt.Errorf("reference file %q: %w", c.reference, err)
		}
		if c.access {
			referenceAccess, err = referenceAccessTime(referenceInfo)
			if err != nil {
				return fmt.Errorf("reference file %q: %w", c.reference, err)
			}
		}
		referenceModification = referenceInfo.ModTime()
	}

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
			if err := f.Close(); err != nil {
				errs = errors.Join(errs, err)
				continue
			}
		}

		accessTime := time.Time{}
		if c.reference != "" {
			if c.access {
				accessTime = referenceAccess
			}
		} else if c.access || notExist {
			accessTime = c.time
		}
		modificationTime := time.Time{}
		if c.reference != "" {
			if c.modification {
				modificationTime = referenceModification
			}
		} else if c.modification || notExist {
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
