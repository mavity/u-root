// Copyright 2025 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lsfd

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestLSFD(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping /proc test on non-linux")
	}

	t.Run("all files", func(t *testing.T) {
		err := run(io.Discard, proc, nil)
		if err != nil {
			t.Errorf("expected nil got %v", err)
		}
	})

	t.Run("one file", func(t *testing.T) {
		err := run(io.Discard, proc, []string{"1"})
		if err != nil {
			t.Errorf("expected nil got %v", err)
		}
	})
}

func TestMain(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}

	// 1. lsfd -help -> exit code 0
	code := Main(context.Background(), ioEnv, "-help")
	if code != 0 {
		t.Fatalf("Main(-help) returned %d, want 0", code)
	}
}
