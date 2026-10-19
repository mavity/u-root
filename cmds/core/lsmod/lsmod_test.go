// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lsmod

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestRun(t *testing.T) {
	t.Run("file don't exists", func(t *testing.T) {
		err := run(nil, "filenotexists")
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %v, got %v", os.ErrNotExist, err)
		}
	})
	t.Run("lsmod", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		err := run(stdout, "./testdata/modules.txt")
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		if !strings.Contains(stdout.String(), "nf_defrag_ipv6") {
			t.Errorf("expected to have nf_defrag_ipv6, got %v", stdout.String())
		}
	})
}

func TestMain(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}

	// 1. lsmod -help -> exit code 0
	code := Main(context.Background(), ioEnv, "-help")
	if code != 0 {
		t.Fatalf("Main(-help) returned %d, want 0", code)
	}
}
