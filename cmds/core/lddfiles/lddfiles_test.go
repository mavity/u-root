// Copyright 2009-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lddfiles

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "file")
	if err != nil {
		t.Fatalf("expected nil got %v", err)
	}
	_ = f.Close()

	stdout := &bytes.Buffer{}

	err = run(stdout, []string{f.Name()})
	if err != nil {
		t.Fatalf("expected nil got %v", err)
	}

	expected := f.Name() + "\n"

	if stdout.String() != expected {
		t.Errorf("expected %q, got %q", expected, stdout.String())
	}
}

func TestMain(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "file2")
	if err != nil {
		t.Fatalf("expected nil got %v", err)
	}
	_ = f.Close()

	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr, Dir: dir}

	// 1. lddfiles -help -> exit code 0
	code := Main(context.Background(), ioEnv, "-help")
	if code != 0 {
		t.Fatalf("Main(-help) returned %d, want 0", code)
	}

	// 2. lddfiles f -> exit code 0, lists file
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, f.Name())
	if code != 0 {
		t.Fatalf("Main(file) returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), f.Name()) {
		t.Errorf("expected %q in stdout, got %q", f.Name(), stdout.String())
	}
}
