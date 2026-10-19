// Copyright 2013-2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xargs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestTooSmall(t *testing.T) {
	p := params{maxArgs: 0}
	ioEnv := core.IO{}
	err := run(context.Background(), ioEnv, p)
	if !errors.Is(err, errTooSmall) {
		t.Errorf("expected %v, got %v", errTooSmall, err)
	}
}

func TestCommandNotFound(t *testing.T) {
	stdin := strings.NewReader("hello world")
	p := params{maxArgs: 1, trace: false}
	ioEnv := core.IO{Stdin: stdin}
	err := run(context.Background(), ioEnv, p, "commandnotfound-xyz-abc", "arg1")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestEcho(t *testing.T) {
	stdin := strings.NewReader("hello world")
	stdout := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout}
	p := params{maxArgs: defaultMaxArgs}
	err := run(context.Background(), ioEnv, p)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	if stdout.String() != "hello world\n" {
		t.Errorf("expected 'hello world', got %q", stdout.String())
	}
}

func TestEchoWithMaxArgs(t *testing.T) {
	stdin := strings.NewReader("a b c d e f g")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}
	p := params{maxArgs: 3, trace: true}
	err := run(context.Background(), ioEnv, p)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if stdout.String() != "a b c\nd e f\ng\n" {
		t.Errorf("expected 'a b c\nd e f\ng\n', got %q", stdout.String())
	}
	expectedStderr := "echo a b c\necho d e f\necho g\n"
	if stderr.String() != expectedStderr {
		t.Errorf("expected %q, got %q", expectedStderr, stderr.String())
	}
}

func TestEchoInteractive(t *testing.T) {
	stdin := strings.NewReader("a b c")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	dir := t.TempDir()
	path := filepath.Join(dir, "tty")
	err := os.WriteFile(path, []byte("yes\nn\ny\n"), 0o644)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}
	p := params{maxArgs: 1, prompt: true, trace: true, tty: path}
	err = run(context.Background(), ioEnv, p)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	if stdout.String() != "a\nc\n" {
		t.Errorf("expected 'a\nc\n' got %q", stdout.String())
	}
}

func TestEchoInteractiveControlD(t *testing.T) {
	stdin := strings.NewReader("a b c")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	dir := t.TempDir()
	path := filepath.Join(dir, "tty")
	err := os.WriteFile(path, []byte("y\n"), 0o644)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}
	p := params{maxArgs: 1, prompt: true, trace: true, tty: path}
	err = run(context.Background(), ioEnv, p)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	if stdout.String() != "a\n" {
		t.Errorf("expected 'a\n' got %q", stdout.String())
	}
}

func TestNullDelimiter(t *testing.T) {
	stdin := strings.NewReader("hello\x00world")
	stdout := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout}
	p := params{maxArgs: defaultMaxArgs, null: true}
	err := run(context.Background(), ioEnv, p)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	if stdout.String() != "hello world\n" {
		t.Errorf("expected 'hello world', got %q", stdout.String())
	}
}
