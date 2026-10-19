// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//go:build !tinygo || tinygo.enable

package ip

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestMain(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}

	// 1. ip -help -> exit code 0
	code := Main(context.Background(), ioEnv, "-help")
	if code != 0 {
		t.Fatalf("Main(-help) returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage: ip") && !strings.Contains(stdout.String(), "Usage: ip") {
		t.Errorf("expected usage output, got stdout=%q, stderr=%q", stdout.String(), stderr.String())
	}

	// 2. ip link -> exit code 0
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, "link")
	if code != 0 {
		t.Fatalf("Main(link) returned %d, want 0; stderr: %s", code, stderr.String())
	}

	// 3. ip addr -> exit code 0
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, "addr")
	if code != 0 {
		t.Fatalf("Main(addr) returned %d, want 0; stderr: %s", code, stderr.String())
	}

	// 4. ip route -> exit code 0
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, "route")
	if code != 0 {
		t.Fatalf("Main(route) returned %d, want 0; stderr: %s", code, stderr.String())
	}

	// 5. ip unknown -> exit code 1
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, "invalidcommand123")
	if code != 1 {
		t.Fatalf("Main(invalidcommand123) returned %d, want 1", code)
	}
}
