// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package pci

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestWindowsPCI(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}

	// 1. pci -h -> exit code 0
	code := Main(context.Background(), ioEnv, "-h")
	if code != 0 {
		t.Fatalf("Main(-h) returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage: pci") {
		t.Errorf("expected usage output, got: %q", stderr.String())
	}

	// 2. pci default -> exit code 0, lists devices
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv)
	if code != 0 {
		t.Fatalf("Main() returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// Verify it printed devices from Windows PCI bus
	out := stdout.String()
	t.Logf("pci output on Windows:\n%s", out)
	if len(strings.TrimSpace(out)) == 0 {
		t.Errorf("pci returned empty output on Windows")
	}

	// 3. pci -n -> exit code 0, numbers only
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, "-n")
	if code != 0 {
		t.Fatalf("Main(-n) returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if len(strings.TrimSpace(stdout.String())) == 0 {
		t.Errorf("pci -n returned empty output on Windows")
	}
}
