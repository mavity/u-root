// Copyright 2017-2019 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package yes implements the yes core utility.
package yes

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"syscall"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

func runYes(w io.Writer, count uint64, args ...string) error {
	yes := "y"
	if len(args) > 0 {
		yes = strings.Join(args, " ")
	}
	// If count == 0 this loop runs to infinity, every other value of count
	// will iterate the loop for exactly "count" times before it breaks out.
	// The standard behavior is achieved with count = 0 as done in main()
	// Sadly this is required to make it testable with the recommended pattern.
	for {
		if _, err := fmt.Fprintf(w, "%s\n", yes); err != nil {
			return err
		}
		if count > 1 {
			count--
		} else if count == 1 {
			break
		}
	}
	return nil
}

// Main runs the yes command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "yes panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("yes", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	line := "y"
	if len(fs.Args()) > 0 {
		line = strings.Join(fs.Args(), " ")
	}
	outLine := []byte(line + "\n")

	for {
		select {
		case <-ctx.Done():
			return 130
		default:
		}
		if _, err := ioEnv.Stdout.Write(outLine); err != nil {
			errMsg := strings.ToLower(err.Error())
			if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, syscall.EPIPE) ||
				strings.Contains(errMsg, "broken pipe") || strings.Contains(errMsg, "pipe is being closed") ||
				strings.Contains(errMsg, "closed pipe") {
				return 0
			}
			fmt.Fprintf(ioEnv.Stderr, "yes: %v\n", err)
			return 1
		}
	}
}
