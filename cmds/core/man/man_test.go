// Copyright 2019 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package man

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u-root/u-root/cmds/core/man/data"
	"github.com/u-root/u-root/pkg/core"
)

func TestMainInvocation(t *testing.T) {
	ctx := context.Background()

	t.Run("missing argument returns 1", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv)
		if code != 1 {
			t.Fatalf("expected code 1 on missing arg, got %d", code)
		}
		if !strings.Contains(stderr.String(), "Usage: man COMMAND") {
			t.Errorf("stderr %q should contain usage", stderr.String())
		}
	})

	t.Run("unknown command returns 1", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "unknown-cmd-xyz")
		if code != 1 {
			t.Fatalf("expected code 1 on unknown command, got %d", code)
		}
		if !strings.Contains(stderr.String(), `No manual entry for "unknown-cmd-xyz"`) {
			t.Errorf("stderr %q should indicate no manual entry", stderr.String())
		}
	})

	t.Run("existing command prints manual entry", func(t *testing.T) {
		oldData := data.Data
		defer func() { data.Data = oldData }()
		data.Data = `{"echo": "echo [args...] - print arguments"}`

		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "echo")
		if code != 0 {
			t.Fatalf("expected code 0, got %d; stderr=%s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "echo [args...] - print arguments") {
			t.Errorf("stdout = %q, expected manual entry", stdout.String())
		}
	})

	t.Run("help flag returns 0", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "-help")
		if code != 0 {
			t.Fatalf("expected code 0 on -help, got %d", code)
		}
	})
}
