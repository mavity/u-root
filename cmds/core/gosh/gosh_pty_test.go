// Copyright 2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Derived work from Daniel Martí <mvdan@mvdan.cc>
//go:build (!tinygo || tinygo.enable) && !plan9 && !windows

package gosh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/u-root/gobusybox/src/pkg/golang"
)

type consoleAction func(*expect.Console) error

func expectOpts(o ...expect.ExpectOpt) consoleAction {
	return func(c *expect.Console) error {
		_, err := c.Expect(o...)
		return err
	}
}

func expectString(s string) consoleAction {
	return func(c *expect.Console) error {
		_, err := c.ExpectString(s)
		if err != nil {
			return fmt.Errorf("failed to expect %q: %w", s, err)
		}
		return nil
	}
}

func send(s string) consoleAction {
	return func(c *expect.Console) error {
		_, err := c.Send(s)
		return err
	}
}

func TestInteractiveBubbline(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "gosh")

	// Setting -cover without GOCOVERDIR adds extra warning output, which changes the result of the test.
	opts := &golang.BuildOpts{ExtraArgs: []string{"-tags=goshbubble"}}
	if os.Getenv("GOCOVERDIR") != "" {
		opts.ExtraArgs = append(opts.ExtraArgs, "-covermode=atomic")
	}
	// Build the stuff.
	if err := golang.Default(golang.DisableCGO()).BuildDir("", execPath, opts); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name   string
		expect []consoleAction
	}{
		{
			name: "exit shell",
			expect: []consoleAction{
				expectString("> "),
				send("echo hi\x0D"),
				expectString("> "),
				send("exit\x0D"),
			},
		},
		{
			name: "source script",
			expect: []consoleAction{
				expectString("> "),
				send("source ./testdata/setenv.sh && echo $FOO\x0D"),
				expectString("hi"),
				expectString("hahaha"),
				expectString("> "),
				send("exit\x0D"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			con, err := expect.NewTestConsole(t, expect.WithDefaultTimeout(2*time.Second), expect.WithStdout(os.Stdout))
			if err != nil {
				t.Fatal(err)
			}

			cmd := exec.CommandContext(context.Background(), execPath)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = con.Tty(), con.Tty(), con.Tty()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// Close our end of child's tty.
			con.Tty().Close()

			for i, a := range tt.expect {
				if err := a(con); err != nil {
					t.Errorf("Action %d: %v", i, err)
				}
			}

			if err := cmd.Wait(); err != nil {
				t.Error(err)
			}
			if err := con.Close(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestInteractiveLiner(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "gosh")

	var opts *golang.BuildOpts
	// Setting -cover without GOCOVERDIR adds extra warning output, which changes the result of the test.
	if os.Getenv("GOCOVERDIR") != "" {
		opts = &golang.BuildOpts{ExtraArgs: []string{"-covermode=atomic"}}
	}
	// Build the stuff.
	if err := golang.Default(golang.DisableCGO(), golang.WithBuildTag("!goshbubble")).BuildDir("", execPath, opts); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name   string
		expect []consoleAction
	}{
		{
			name: "exit shell",
			expect: []consoleAction{
				expectString("$ "),
				send("echo hi\x0D"),
				expectString("hi"),
				expectString("$ "),
				send("exit\x0D"),
			},
		},
		{
			name: "source script",
			expect: []consoleAction{
				expectString("$ "),
				send("source ./testdata/setenv.sh && echo $FOO\x0D"),
				expectString("hi"),
				expectString("hahaha"),
				expectString("$ "),
				send("exit\x0D"),
			},
		},
		{
			name: "child reads stdin",
			expect: []consoleAction{
				expectString("$ "),
				// "echo READY" ensures stopPrompt() has been called and the
				// terminal is back in canonical mode before we send the stdin
				// input for the subsequent "read" builtin.
				send("echo READY && read X && echo \"got: $X\"\x0D"),
				expectString("READY"),
				send("hello\x0D"),
				expectString("got: hello"),
				expectString("$ "),
				send("exit\x0D"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			con, err := expect.NewTestConsole(t, expect.WithDefaultTimeout(2*time.Second))
			if err != nil {
				t.Fatal(err)
			}

			cmd := exec.CommandContext(context.Background(), execPath)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = con.Tty(), con.Tty(), con.Tty()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// Close our end of child's tty.
			con.Tty().Close()

			for i, a := range tt.expect {
				if err := a(con); err != nil {
					t.Errorf("Action %d: %v", i, err)
				}
			}

			if err := cmd.Wait(); err != nil {
				t.Error(err)
			}
			if err := con.Close(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestGoshInvocation(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "gosh")
	var opts *golang.BuildOpts
	// Setting -cover without GOCOVERDIR adds extra warning output, which changes the result of the test.
	if os.Getenv("GOCOVERDIR") != "" {
		opts = &golang.BuildOpts{ExtraArgs: []string{"-covermode=atomic"}}
	}
	// Build the stuff.
	if err := golang.Default(golang.DisableCGO()).BuildDir("", execPath, opts); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name       string
		args       []string
		stdout     string
		stdin      string
		exitStatus int
	}{
		{
			name: "echo",
			args: []string{
				"-c", "echo foo",
			},
			stdout: "foo\n",
		},
		{
			name: "cmdline cmd wins",
			args: []string{
				"-c", "echo foo", "./testdata/setenv.sh",
			},
			stdout: "foo\n",
		},
		{
			name: "cmd",
			args: []string{
				"./testdata/setenv.sh",
			},
			stdout: "hi\n",
		},
		{
			name:   "stdin",
			stdin:  "echo hi",
			stdout: "hi\n",
		},
		{
			name:       "exit status",
			stdin:      "false",
			exitStatus: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.CommandContext(context.Background(), execPath, tt.args...)
			var b bytes.Buffer
			if tt.stdin == "" {
				// This means os.Stdin would register as a terminal.
				con, err := expect.NewTestConsole(t, expect.WithDefaultTimeout(2*time.Second))
				if err != nil {
					t.Fatal(err)
				}
				cmd.Stdin = con.Tty()
				t.Cleanup(func() { con.Close() })
			} else {
				cmd.Stdin = strings.NewReader(tt.stdin)
			}
			cmd.Stdout, cmd.Stderr = &b, &b
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}

			var e *exec.ExitError
			if err := cmd.Wait(); errors.As(err, &e) {
				if got := e.Sys().(syscall.WaitStatus).ExitStatus(); got != tt.exitStatus {
					t.Errorf("Exit status = %v, want %d", err, tt.exitStatus)
				}
			} else if err != nil {
				t.Error(err)
			}
			if got := b.String(); got != tt.stdout {
				t.Errorf("gosh = %s, want %s", got, tt.stdout)
			}
		})
	}
}

func TestInteractiveSimple(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "gosh")

	var opts *golang.BuildOpts
	// Setting -cover without GOCOVERDIR adds extra warning output, which changes the result of the test.
	if os.Getenv("GOCOVERDIR") != "" {
		opts = &golang.BuildOpts{ExtraArgs: []string{"-covermode=atomic"}}
	}
	// Build the stuff.
	if err := golang.Default(golang.DisableCGO(), golang.WithBuildTag("goshsmall")).BuildDir("", execPath, opts); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name   string
		expect []consoleAction
	}{
		{
			name: "exit shell",
			expect: []consoleAction{
				expectString("$ "),
				send("echo hi\x0D"),
				expectString("hi"),
				expectString("$ "),
				send("exit\x0D"),
			},
		},
		{
			name: "source script",
			expect: []consoleAction{
				expectString("$ "),
				send("source ./testdata/setenv.sh && echo $FOO\x0D"),
				expectString("hi"),
				expectString("hahaha"),
				expectString("$ "),
				send("exit\x0D"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			con, err := expect.NewTestConsole(t, expect.WithDefaultTimeout(2*time.Second))
			if err != nil {
				t.Fatal(err)
			}

			cmd := exec.CommandContext(context.Background(), execPath)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = con.Tty(), con.Tty(), con.Tty()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// Close our end of child's tty.
			con.Tty().Close()

			for i, a := range tt.expect {
				if err := a(con); err != nil {
					t.Errorf("Action %d: %v", i, err)
				}
			}

			if err := cmd.Wait(); err != nil {
				t.Error(err)
			}
			if err := con.Close(); err != nil {
				t.Error(err)
			}
		})
	}
}
