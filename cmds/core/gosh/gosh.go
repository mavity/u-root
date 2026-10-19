// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Derived work from Daniel Martí <mvdan@mvdan.cc>

//go:build (!tinygo || tinygo.enable) && !plan9

package gosh

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/u-root/u-root/pkg/core"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// HistFile is the history file.
var HistFile = filepath.Join(os.TempDir(), "gosh.history")

var errNotImplemented = errors.New("fancy interactive interpreter not implemented")

// Main runs gosh with the given context, I/O environment, and arguments.
func Main(ctx context.Context, ioEnv core.IO, args ...string) int {
	if err := runWithIO(ctx, ioEnv, args...); err != nil {
		if status, ok := interp.IsExitStatus(err); ok {
			return int(status)
		}
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(ioEnv.Stderr, "gosh: %v\n", err)
		return 1
	}
	return 0
}

func runWithIO(ctx context.Context, ioEnv core.IO, args ...string) error {
	fs := flag.NewFlagSet("gosh", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var cmdStr string
	fs.StringVar(&cmdStr, "c", "", "Command to run")

	parseArgs := args
	if len(parseArgs) > 0 && !strings.HasPrefix(parseArgs[0], "-") {
		if parseArgs[0] == "gosh" || filepath.Base(parseArgs[0]) == "gosh" {
			parseArgs = parseArgs[1:]
		}
	}

	if err := fs.Parse(parseArgs); err != nil {
		return err
	}

	stdin := ioEnv.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	stdout := ioEnv.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := ioEnv.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	opts := []interp.RunnerOption{
		interp.StdIO(stdin, stdout, stderr),
	}
	if ioEnv.Dir != "" {
		opts = append(opts, interp.Dir(ioEnv.Dir))
	}

	runner, err := interp.New(opts...)
	if err != nil {
		return err
	}

	if cmdStr != "" {
		return runReaderCtx(ctx, runner, strings.NewReader(cmdStr), "")
	}
	if fs.NArg() == 0 {
		return runReaderCtx(ctx, runner, stdin, "")
	}
	return runScriptCtx(ctx, runner, ioEnv.ResolvePath(fs.Arg(0)))
}

func run(stdin io.Reader, stdout, stderr io.Writer, command string, args ...string) error {
	return runCtx(context.Background(), stdin, stdout, stderr, command, args...)
}

func runCtx(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, command string, args ...string) error {
	runner, err := interp.New(interp.StdIO(stdin, stdout, stderr))
	if err != nil {
		return err
	}

	if command != "" {
		return runReaderCtx(ctx, runner, strings.NewReader(command), "")
	}
	if len(args) == 0 {
		if r, ok := stdin.(*os.File); ok && term.IsTerminal(int(r.Fd())) {
			if err := runInteractive(runner, syntax.NewParser(), stdout, stderr); !errors.Is(err, errNotImplemented) {
				return err
			}
			return runInteractiveSimple(runner, stdin, stdout)
		}
		return runReaderCtx(ctx, runner, stdin, "")
	}
	return runScriptCtx(ctx, runner, args[0])
}

func runScript(runner *interp.Runner, file string) error {
	return runScriptCtx(context.Background(), runner, file)
}

func runScriptCtx(ctx context.Context, runner *interp.Runner, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return runReaderCtx(ctx, runner, f, file)
}

func runReader(runner *interp.Runner, reader io.Reader, name string) error {
	return runReaderCtx(context.Background(), runner, reader, name)
}

func runReaderCtx(ctx context.Context, runner *interp.Runner, reader io.Reader, name string) error {
	prog, err := syntax.NewParser().Parse(reader, name)
	if err != nil {
		return err
	}
	runner.Reset()
	return runner.Run(ctx, prog)
}

func runInteractiveSimple(runner *interp.Runner, stdin io.Reader, stdout io.Writer) error {
	parser := syntax.NewParser()
	fmt.Fprintf(stdout, "$ ")

	var runErr error
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func(ch chan os.Signal) {
		for {
			<-ch
		}
	}(ch)

	for {
		fn := func(stmts []*syntax.Stmt) bool {
			if parser.Incomplete() {
				fmt.Fprintf(stdout, "> ")
				return true
			}
			for _, stmt := range stmts {
				runErr = runner.Run(context.Background(), stmt)
				if runner.Exited() {
					return false
				}
			}
			fmt.Fprintf(stdout, "$ ")
			return true
		}

		if err := parser.Interactive(stdin, fn); err != nil {
			return err
		}

		if runErr != nil {
			fmt.Fprintf(stdout, "error: %s\n", runErr.Error())
			runErr = nil
		} else {
			return nil
		}
	}
}
