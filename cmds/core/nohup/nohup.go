// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//go:build !tinygo || tinygo.enable

// Package nohup invokes a utility immune to hangups.
//
// Synopsis:
//
//	nohup <command> [args...]
package nohup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/u-root/u-root/pkg/core"
	"golang.org/x/term"
)

var (
	errUsage  = fmt.Errorf("nohup <command> [args...]")
	errStart  = fmt.Errorf("failed to start")
	errFinish = fmt.Errorf("finished with error")
)

func run(args []string) error {
	ioEnv := core.IO{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	return runWithIO(context.Background(), ioEnv, args)
}

func runWithIO(ctx context.Context, ioEnv core.IO, args []string) error {
	if len(args) < 2 {
		return errUsage
	}

	ignoreSIGHUP()

	cmdName := args[1]
	cmdArgs := args[2:]

	stdoutIsTerminal := false
	if f, ok := ioEnv.Stdout.(interface{ Fd() uintptr }); ok {
		stdoutIsTerminal = term.IsTerminal(int(f.Fd()))
	}
	stderrIsTerminal := false
	if f, ok := ioEnv.Stderr.(interface{ Fd() uintptr }); ok {
		stderrIsTerminal = term.IsTerminal(int(f.Fd()))
	}

	childIO := ioEnv

	if stdoutIsTerminal {
		outPath := ioEnv.ResolvePath("nohup.out")
		outputFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("error opening file: %w", err)
		}
		defer outputFile.Close()

		childIO.Stdout = outputFile
		if stderrIsTerminal {
			childIO.Stderr = outputFile
		}
	}

	if ioEnv.Runner != nil {
		allArgs := append([]string{cmdName}, cmdArgs...)
		code := ioEnv.Runner(ctx, childIO, allArgs...)
		if code != 0 {
			return fmt.Errorf("%s: %w: exit code %d", cmdName, errFinish, code)
		}
		return nil
	}

	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	cmd.Stdin = childIO.Stdin
	cmd.Stdout = childIO.Stdout
	cmd.Stderr = childIO.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w: %w", cmdName, errStart, err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s: %w: %w", cmdName, errFinish, err)
	}

	return nil
}

// Main runs the nohup command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "nohup: panic: %v\n", r)
			exitCode = 1
		}
	}()

	var runArgs []string
	if len(args) > 0 && args[0] == "nohup" {
		runArgs = args
	} else {
		runArgs = append([]string{"nohup"}, args...)
	}

	if err := runWithIO(ctx, ioEnv, runArgs); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintf(ioEnv.Stderr, "Usage: %v\n", errUsage)
			return 127
		}
		if errors.Is(err, errStart) {
			fmt.Fprintf(ioEnv.Stderr, "nohup: %v\n", err)
			return 127
		}
		fmt.Fprintf(ioEnv.Stderr, "nohup: %v\n", err)
		return 126
	}
	return 0
}
