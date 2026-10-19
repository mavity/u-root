// SPDX-License-Identifier: MIT
// Copyright 2026 Google LLC

package ufs

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hugelgupf/p9/p9"
	"github.com/u-root/u-root/pkg/core"
)

func TestUfsUsageAndFlags(t *testing.T) {
	var stderr bytes.Buffer
	code := Main(context.Background(), core.IO{Stderr: &stderr}, "-help")
	if code != 0 {
		t.Errorf("expected 0 for -help, got %d", code)
	}

	stderr.Reset()
	code = Main(context.Background(), core.IO{Stderr: &stderr})
	if code != 1 {
		t.Errorf("expected 1 for missing arguments, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("expected usage message, got %q", stderr.String())
	}
}

func TestUfsServeAndCancel(t *testing.T) {
	// Find a free TCP port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	root := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var serverStderr bytes.Buffer
	serverDone := make(chan int, 1)
	go func() {
		serverDone <- Main(ctx, core.IO{Stderr: &serverStderr}, "-root", root, addr)
	}()

	// Connect to 9P server with retries
	var conn net.Conn
	for i := 0; i < 20; i++ {
		conn, err = net.Dial("tcp", addr)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("failed to dial ufs at %s: %v (server stderr: %s)", addr, err, serverStderr.String())
	}
	defer conn.Close()

	// 9P client attach and test
	clt, err := p9.NewClient(conn)
	if err != nil {
		t.Fatalf("p9.NewClient failed: %v", err)
	}

	rootFile, err := clt.Attach("/")
	if err != nil {
		t.Fatalf("clt.Attach failed: %v", err)
	}
	_ = rootFile.Close()

	// Cancel context and verify clean server shutdown
	cancel()
	select {
	case code := <-serverDone:
		if code != 0 {
			t.Errorf("ufs server exited with code %d, want 0", code)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("ufs server did not stop after context cancel")
	}
}
