// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dmesg

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestMainHelp(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}

	code := Main(context.Background(), ioEnv, "-help")
	if code != 0 {
		t.Fatalf("Main(-help) returned %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "Usage of dmesg") && !strings.Contains(stderr.String(), "Usage: dmesg") {
		t.Errorf("expected Usage in stderr, got %q", stderr.String())
	}
}

func TestMainExecution(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}

	// Should not panic on any platform
	code := Main(context.Background(), ioEnv)
	t.Logf("Main() returned code=%d, stdout len=%d, stderr=%s", code, stdout.Len(), stderr.String())
}
