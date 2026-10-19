// Copyright 2019 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// man - print manual entry for command.
//
// Synopsis:
//
//	man COMMAND
package man

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"

	"github.com/u-root/u-root/cmds/core/man/data"
	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

// Main prints the manual entry for a command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "man panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("man", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintf(ioEnv.Stderr, "Usage: man COMMAND\n")
		return 1
	}

	mans := make(map[string]string)
	if err := json.Unmarshal([]byte(data.Data), &mans); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "man: %v\n", err)
		return 1
	}

	cmd := rest[0]
	man, ok := mans[cmd]
	if !ok {
		fmt.Fprintf(ioEnv.Stderr, "No manual entry for %q\n", cmd)
		return 1
	}

	_, _ = fmt.Fprintln(ioEnv.Stdout, man)
	return 0
}
