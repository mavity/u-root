// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package gosh

import (
	"bytes"
	"io"
	"syscall"
	"testing"
)

func TestUmaskBuiltin(t *testing.T) {
	oldMask := syscall.Umask(0)
	syscall.Umask(oldMask)
	t.Cleanup(func() { syscall.Umask(oldMask) })

	in, writer := io.Pipe()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run(in, &stdout, &stderr, "umask 0077; umask; umask -S; umask -p"); err != nil {
		t.Fatalf("run umask builtin: %v; stderr=%q", err, stderr.String())
	}
	want := "0077\nu=rwx,g=,o=\numask 0077\n"
	if got := stdout.String(); got != want {
		t.Fatalf("umask output = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("umask stderr = %q, want empty", stderr.String())
	}
}
