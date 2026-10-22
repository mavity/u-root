// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Truncate - shrink or extend the size of a file to the specified size
//
// Synopsis:
//
//	truncate [OPTIONS] [FILE]...
//
// Options:
//
//	-s: size in bytes
//	-o: treat SIZE as number of IO blocks instead of bytes (1 block = 512 bytes)
//	-r: reference file for size
//	-c: do not create any files
//
// Author:
//
//	Roland Kammerer <dev.rck@gmail.com>
package truncate

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/rck/unit"
	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
	"github.com/u-root/u-root/pkg/uroot/util"
)

const usage = "truncate [-c] -s size file..."

const ioBlockSize = 512

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var noCreate, ioBlocks bool
	var rfile string
	size := unit.MustNewUnit(unit.DefaultUnits).MustNewValue(1, unit.None)

	fs := flag.NewFlagSet("truncate", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&noCreate, "c", false, "Do not create files.")
	fs.BoolVar(&ioBlocks, "o", false, "Treat SIZE as number of IO blocks instead of bytes.")
	fs.Var(size, "s", "Size in bytes, prefixes +/- are allowed")
	fs.StringVar(&rfile, "r", "", "Reference file for size")
	fs.Usage = util.Usage(fs.Usage, usage)

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	if (!size.IsSet && rfile == "") || (size.IsSet && rfile != "") {
		return fmt.Errorf("you need to specify size via -s <number> or -r <rfile>")
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("you need to specify one or more files as argument")
	}

	for _, fname := range rest {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resolved := ioEnv.ResolvePath(fname)
		st, err := os.Stat(resolved)
		if os.IsNotExist(err) && !noCreate {
			if err = os.WriteFile(resolved, []byte{}, 0o644); err != nil {
				return fmt.Errorf("%w", err)
			}
			if st, err = os.Stat(resolved); err != nil {
				return fmt.Errorf("could not stat newly created file: %w", err)
			}
		} else if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("could not stat %q: %w", resolved, err)
		}
		var final int64
		if rfile != "" {
			refResolved := ioEnv.ResolvePath(rfile)
			if st, err = os.Stat(refResolved); err != nil {
				return fmt.Errorf("could not stat reference file: %w", err)
			}
			final = st.Size()
		} else if size.IsSet {
			final = size.Value // base case
			if ioBlocks {
				final *= ioBlockSize
			}
			if size.ExplicitSign != unit.None {
				if st != nil {
					final += st.Size() // in case of '-', size.Value is already negative
				}
			}
			if final < 0 {
				final = 0
			}
		}

		if err := os.Truncate(resolved, final); err != nil {
			return fmt.Errorf("could not truncate %q: %w", resolved, err)
		}
	}
	return nil
}

// Main runs the truncate command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "truncate: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "truncate: %v\n", err)
		}
		return 1
	}
	return 0
}
