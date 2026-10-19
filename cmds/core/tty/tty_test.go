// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tty

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestTTY(t *testing.T) {
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("hello")
	_ = run(stdout, stdin, false)
	// Even if not a tty, stdout has output ("not a tty" or device name)
	if stdout.Len() == 0 {
		t.Errorf("expected stdout output, got none")
	}
}

func TestMain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{
		Stdin:  strings.NewReader("not a console"),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// 1. tty with non-tty stdin returns 1
	code := Main(context.Background(), ioEnv)
	if code != 1 && code != 0 {
		t.Fatalf("Main unexpected exit code: %d", code)
	}

	// 2. tty -s (silent mode)
	stdout.Reset()
	code = Main(context.Background(), ioEnv, "-s")
	if code != 1 && code != 0 {
		t.Fatalf("Main(-s) unexpected exit code: %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Main(-s) printed output: %q", stdout.String())
	}

	// 3. tty -help returns 0
	code = Main(context.Background(), ioEnv, "-help")
	if code != 0 {
		t.Fatalf("Main(-help) returned %d", code)
	}
}
