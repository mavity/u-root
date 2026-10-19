// Copyright 2015-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package df

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestRunDF(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		fargs   flags
		wantErr error
	}{
		{
			name: "No-such-file-or-directory",
			args: []string{""},
		},
		{
			name: "NoArgs-NoFlags",
		},
		{
			name: "NoArgs-M-Flag",
			fargs: flags{
				m: true,
			},
		},
		{
			name: "NoArgs-K-Flag",
			fargs: flags{
				k: true,
			},
		},
		{
			name: "NoArgs-KM-Flag",
			fargs: flags{
				k: true,
				m: true,
			},
			wantErr: errKMExclusiv,
		},
		{
			name: "Dir as argument",
			args: []string{os.TempDir()},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := df(&buf, tt.fargs, tt.args); !errors.Is(err, tt.wantErr) {
				t.Errorf("df(%v, %v, buf)=%q, want %q", tt.args, tt.fargs, err, tt.wantErr)
			}
		})
	}
}

func TestMain(t *testing.T) {
	t.Run("default df", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr})
		if code != 0 {
			t.Fatalf("expected code 0, got %d, stderr=%s", code, stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "Filesystem") || !strings.Contains(out, "1K-blocks") {
			t.Errorf("header missing: %s", out)
		}
	})

	t.Run("flag -m", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr}, "-m")
		if code != 0 {
			t.Fatalf("df -m failed: %d, stderr=%s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "1M-blocks") {
			t.Errorf("expected 1M-blocks header: %s", stdout.String())
		}
	})

	t.Run("flag -k and -m conflict", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr}, "-k", "-m")
		if code != 1 {
			t.Fatalf("df -k -m expected code 1, got %d", code)
		}
	})

	t.Run("temp dir arg", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr}, os.TempDir())
		if code != 0 {
			t.Fatalf("df tempdir failed: %d, stderr=%s", code, stderr.String())
		}
	})

	t.Run("flag -help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr}, "-help")
		if code != 0 {
			t.Fatalf("df -help expected 0, got %d", code)
		}
	})
}
