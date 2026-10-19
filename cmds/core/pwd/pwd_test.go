// Copyright 2012-2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pwd

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestPWD(t *testing.T) {
	tests := []struct {
		name     string
		physical bool
	}{
		{
			name:     "follow-symlinks",
			physical: true,
		},
		{
			name:     "no-follow-symlinks",
			physical: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			expected := dir
			if tt.physical {
				var err error
				expected, err = filepath.EvalSymlinks(dir)
				if err != nil {
					t.Fatalf("filepath.EvalSymlinks(%q): got %v, want nil", dir, err)
				}
			}

			ioEnv := core.IO{Dir: dir}
			res, err := pwd(ioEnv, tt.physical)
			if err != nil {
				t.Fatalf("pwd(%t): error got %v, want nil", tt.physical, err)
			}
			if res != expected {
				t.Errorf("pwd(%t): got %q, want %q", tt.physical, res, expected)
			}

			var stdout bytes.Buffer
			ioEnv.Stdout = &stdout
			ioEnv.Stderr = io.Discard
			var args []string
			if tt.physical {
				args = []string{"-P"}
			} else {
				args = []string{"-L"}
			}
			if err := run(context.Background(), ioEnv, args...); err != nil {
				t.Fatalf("run(%v): error got %v", args, err)
			}
			if strings.TrimSpace(stdout.String()) != expected {
				t.Errorf("run(%v): got %q, want %q", args, strings.TrimSpace(stdout.String()), expected)
			}
		})
	}
}
