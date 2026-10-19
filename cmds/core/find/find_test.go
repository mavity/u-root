// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package findcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/find"
)

func create(t *testing.T, name string) {
	t.Helper()
	f, err := os.Create(name)
	if err != nil {
		t.Fatalf("creating test file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing test file: %v", err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		t.Fatalf("setting test file mode: %v", err)
	}
}

func prepareDirLayout(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for _, n := range []string{"1", "2"} {
		if err := os.Mkdir(filepath.Join(d, n), os.ModePerm); err != nil {
			t.Fatal(err)
		}
	}

	for _, n := range []string{"file1", "file2", filepath.Join("1", "file1"), filepath.Join("1", "file2"), filepath.Join("2", "file1"), filepath.Join("2", "file3")} {
		create(t, filepath.Join(d, n))
	}
	return d
}

func TestFind(t *testing.T) {
	dir := prepareDirLayout(t)

	var tests = []struct {
		wantStdout []string
		commandErr error
		runErr     error
		args       []string
	}{
		{
			wantStdout: []string{"file1"},
			args:       []string{"file1"},
		},
		{
			wantStdout: []string{"1", "1/file1", "1/file2"},
			args:       []string{"1"},
		},
		{
			wantStdout: []string{"1/file1", "2/file1", "file1"},
			args:       []string{"-name=file1", "."},
		},
		{
			wantStdout: []string{".", "1", "2"},
			args:       []string{"-type=d", "."},
		},
		{
			wantStdout: []string{".", "1", "2"},
			args:       []string{"-type=directory", "."},
		},
		{
			wantStdout: []string{"1/file1", "1/file2", "2/file1", "2/file3", "file1", "file2"},
			args:       []string{"-type=f", "."},
		},
		{
			wantStdout: []string{"1/file1", "1/file2", "2/file1", "2/file3", "file1", "file2"},
			args:       []string{"-type=file", "."},
		},
		{
			wantStdout: []string{"1/file1", "1/file2", "2/file1", "file1", "file2"},
			args:       []string{"-regex=file[12]", "."},
		},
		{
			wantStdout: []string{"1/file2", "file2"},
			args:       []string{"-regex=file2", "."},
		},
		{
			wantStdout: []string{"1/file1", "2/file1", "file1"},
			args:       []string{"-regex=file1", "."},
		},
		{
			wantStdout: []string{"1/file1", "2/file1", "file1"},
			args:       []string{"-name=file1", "-regex=file[12]", "."},
		},
		{
			args:       []string{"-regex=[", "."},
			commandErr: find.ErrInvalidRegexp,
		},
		{
			args:       []string{"-type=notvalid", "."},
			commandErr: errNotValidType,
		},
		{
			args:       []string{"-mode=0644"},
			commandErr: errUsage,
		},
	}

	for i, tt := range tests {
		var stdout bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: io.Discard, Dir: dir}
		c, err := command(ioEnv, tt.args...)
		if !errors.Is(err, tt.commandErr) {
			t.Errorf("test %d expected %v, got %v", i, tt.commandErr, err)
			continue
		}
		if err != nil {
			continue
		}

		err = c.run(context.Background(), ioEnv)
		if !errors.Is(err, tt.runErr) {
			t.Errorf("test %d expected %v, got %v", i, tt.runErr, err)
			continue
		}
		if err != nil {
			continue
		}

		rawLines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		var lines []string
		for _, l := range rawLines {
			if l != "" {
				lines = append(lines, strings.ReplaceAll(strings.TrimSpace(l), "\\", "/"))
			}
		}

		if len(lines) != len(tt.wantStdout) {
			t.Errorf("test %d count mismatch: want %d lines, got %d lines (%v)", i, len(tt.wantStdout), len(lines), lines)
			continue
		}
		for j := range lines {
			if lines[j] != tt.wantStdout[j] {
				t.Errorf("test %d line %d mismatch: want %q, got %q", i, j, tt.wantStdout[j], lines[j])
			}
		}
	}
}

func TestFindLong(t *testing.T) {
	dir := prepareDirLayout(t)

	var stdout bytes.Buffer
	ioEnv := core.IO{Stdout: &stdout, Stderr: io.Discard, Dir: dir}
	c, err := command(ioEnv, "-l", "file1")
	if err != nil {
		t.Fatal(err)
	}

	err = c.run(context.Background(), ioEnv)
	if err != nil {
		t.Fatal(err)
	}

	res := strings.TrimSpace(stdout.String())
	if !strings.HasSuffix(res, "file1") {
		t.Errorf("want suffix: file1, got: %s", res)
	}
}
