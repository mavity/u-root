// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package netstat

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestWindowsNetstat(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}

	// 1. netstat -h -> exit code 0
	code := Main(context.Background(), ioEnv, "-h")
	if code != 0 {
		t.Fatalf("Main(-h) returned %d, want 0; stderr: %s", code, stderr.String())
	}

	// 2. netstat -i -> exit code 0
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, "-i")
	if code != 0 {
		t.Fatalf("Main(-i) returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Iface") {
		t.Errorf("expected 'Iface' in netstat -i, got: %q", stdout.String())
	}

	// 3. netstat -r -> exit code 0
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, "-r")
	if code != 0 {
		t.Fatalf("Main(-r) returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Destination") {
		t.Errorf("expected 'Destination' in netstat -r, got: %q", stdout.String())
	}

	// 4. netstat -a -> exit code 0, lists sockets
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, "-a")
	if code != 0 {
		t.Fatalf("Main(-a) returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Active Internet connections") {
		t.Errorf("expected header in netstat -a, got: %q", stdout.String())
	}
}
