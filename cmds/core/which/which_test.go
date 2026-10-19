// Copyright 2016 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package which

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestWhich(t *testing.T) {
	tmpDir := t.TempDir()
	cmd1 := filepath.Join(tmpDir, "mycmd1")
	if err := os.WriteFile(cmd1, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{
		Stdout: &stdout,
		Stderr: &stderr,
		LookupEnv: func(k string) (string, bool) {
			if k == "PATH" {
				return tmpDir, true
			}
			return "", false
		},
	}

	if code := Main(context.Background(), ioEnv, "mycmd1"); code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "mycmd1") {
		t.Fatalf("expected stdout to contain mycmd1, got: %q", stdout.String())
	}

	// Missing command
	stdout.Reset()
	stderr.Reset()
	if code := Main(context.Background(), ioEnv, "nonexistent-cmd-xyz"); code == 0 {
		t.Fatalf("expected non-zero exit for nonexistent command")
	}
}
