// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// lsmod lists currently loaded Linux kernel modules.
//
// Synopsis:
//
//	lsmod
//
// Description:
//
//	lsmod is a clone of lsmod(8)
package lsmod

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/u-root/u-root/pkg/core"
)

func run(stdout io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintln(stdout, "Module                  Size  Used by")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		s := strings.Split(scanner.Text(), " ")
		if len(s) < 4 {
			continue
		}
		name, size, used, usedBy := s[0], s[1], s[2], s[3]
		final := fmt.Sprintf("%-19s %8s  %s", name, size, used)
		if usedBy != "-" {
			usedBy = usedBy[:len(usedBy)-1]
			final += fmt.Sprintf(" %s", usedBy)
		}
		fmt.Fprintln(stdout, final)
	}

	return scanner.Err()
}

// Main executes the lsmod command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "lsmod: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	_ = ctx
	fs := flag.NewFlagSet("lsmod", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	fs.Usage = func() {
		fmt.Fprintln(ioEnv.Stderr, "Usage: lsmod")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	path := "/proc/modules"
	if err := run(ioEnv.Stdout, path); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "lsmod: %v\n", err)
		return 1
	}
	return 0
}
