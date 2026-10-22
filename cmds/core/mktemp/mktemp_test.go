// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mktemp

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

type test struct {
	cmdline []string
	out     string
	err     error
}

func TestMkTemp(t *testing.T) {
	tmpDir := os.TempDir()
	tests := []test{
		{
			cmdline: []string{},
			out:     tmpDir,
			err:     nil,
		},
		{
			cmdline: []string{"-d"},
			out:     tmpDir,
			err:     nil,
		},
		{
			cmdline: []string{"foofoo.XXXX"},
			out:     filepath.Join(tmpDir, "foofoo"),
			err:     nil,
		},
		{
			cmdline: []string{"--suffix", "baz", "foo.XXXX"},
			out:     filepath.Join(tmpDir, "foo.baz"),
			err:     nil,
		},
		{
			cmdline: []string{"-u"},
			out:     tmpDir,
			err:     nil,
		},
		{
			cmdline: []string{"-p", "/notexists"},
			out:     "",
			err:     os.ErrNotExist,
		},
		{
			cmdline: []string{"-q", "-p", "/notexists"},
			out:     "",
			err:     os.ErrNotExist,
		},
	}

	// Table-driven testing
	for _, tt := range tests {
		var stdout bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: io.Discard}
		err := run(context.Background(), ioEnv, tt.cmdline...)
		if !errors.Is(err, tt.err) {
			t.Errorf("expected %v, got %v", tt.err, err)
		}

		r := stdout.String()
		if !strings.HasPrefix(r, tt.out) {
			t.Errorf("stdout got:\n%s\nwant starting with:\n%s", r, tt.out)
		}
	}
}

func TestDryRunPrintsUncreatedName(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}

	if err := run(context.Background(), ioEnv, "-u", "-p", dir, "candidate"); err != nil {
		t.Fatalf("run(-u) error = %v", err)
	}

	name := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(name, filepath.Join(dir, "candidate")) {
		t.Fatalf("dry-run name = %q, want prefix %q", name, filepath.Join(dir, "candidate"))
	}
	if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry-run created %q: stat error = %v", name, err)
	}
	if !strings.Contains(stderr.String(), "unsafe") {
		t.Errorf("dry-run warning = %q, want unsafe warning", stderr.String())
	}
}

func TestTmpdirTemplate(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	ioEnv := core.IO{
		Stdout: &stdout,
		Stderr: io.Discard,
		LookupEnv: func(key string) (string, bool) {
			return dir, key == "TMPDIR"
		},
	}

	if err := run(context.Background(), ioEnv, "-t", "sample"); err != nil {
		t.Fatalf("run(-t) error = %v", err)
	}

	name := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(name, filepath.Join(dir, "sample")) {
		t.Fatalf("name = %q, want prefix %q", name, filepath.Join(dir, "sample"))
	}
	if _, err := os.Stat(name); err != nil {
		t.Errorf("mktemp -t did not create %q: %v", name, err)
	}
}

func TestQuietFailureSuppressesDiagnostic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}

	if code := Main(context.Background(), ioEnv, "-q", "-p", dir); code != 1 {
		t.Fatalf("Main(-q) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty output", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want suppressed diagnostic", stderr.String())
	}
}

func TestDefaultFlags(t *testing.T) {
	f := flags{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f.register(fs)

	if f.d {
		t.Error("directory should be false by default")
	}
	if f.u {
		t.Error("dry-run should be false by default")
	}
	if f.q {
		t.Error("quiet should be false by default")
	}
	if f.prefix != "" {
		t.Error("prefix should be empty string by default")
	}
	if f.suffix != "" {
		t.Error("suffix should be empty string by default")
	}
	if f.dir != "" {
		t.Error("tmpdir should be empty string by default")
	}
	if f.t {
		t.Error("tmpdir template mode should be false by default")
	}
}
