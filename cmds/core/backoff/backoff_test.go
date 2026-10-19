// Copyright 2021-2025 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package backoff

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/u-root/u-root/pkg/core"
)

func TestRunIt(t *testing.T) {
	for _, tt := range []struct {
		name    string
		timeout time.Duration
		cmd     string
		args    []string
		wantErr error
	}{
		{
			name:    "noCmd",
			timeout: 3 * time.Second,
			cmd:     "",
			wantErr: errNoCmd,
		},
		{
			name:    "withRunner",
			timeout: 3 * time.Second,
			cmd:     "mycmd",
			args:    []string{"hi"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			ioEnv := core.IO{
				Stdout: &stdout,
				Stderr: &stderr,
			}
			if tt.name == "withRunner" {
				ioEnv.Runner = func(ctx context.Context, subIO core.IO, args ...string) int {
					return 0
				}
			}

			var fullArgs []string
			if tt.cmd != "" {
				fullArgs = append(fullArgs, tt.cmd)
				fullArgs = append(fullArgs, tt.args...)
			}
			err := run(context.Background(), ioEnv, tt.timeout, false, fullArgs...)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("run got %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("run failed: %v", err)
			}
		})
	}
}

func TestBackoffMain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	calls := 0
	ioEnv := core.IO{
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: func(ctx context.Context, subIO core.IO, args ...string) int {
			calls++
			if calls >= 2 {
				return 0
			}
			return 1
		},
	}

	code := Main(context.Background(), ioEnv, "-t", "2s", "testcmd")
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr: %s", code, stderr.String())
	}
	if calls < 2 {
		t.Fatalf("expected at least 2 calls, got %d", calls)
	}
}
