// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stty

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestMain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}

	// 1. Default stty prints state
	code := Main(context.Background(), ioEnv)
	if code != 0 {
		t.Fatalf("Main() returned %d, stderr: %s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatalf("Main() returned empty output")
	}

	// 2. stty dump produces JSON
	stdout.Reset()
	code = Main(context.Background(), ioEnv, "dump")
	if code != 0 {
		t.Fatalf("Main(dump) returned %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "{") {
		t.Fatalf("Main(dump) did not produce JSON: %q", stdout.String())
	}
}
