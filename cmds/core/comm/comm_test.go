// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package comm

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
)

func TestComm(t *testing.T) {
	tmpdir := t.TempDir()
	for _, tt := range []struct {
		name  string
		args  []string
		file1 string
		file2 string
		s1    bool
		s2    bool
		s3    bool
		help  bool
		err   error
		want  string
	}{
		{
			name: "only one argument",
			args: []string{"onearg"},
			err:  ErrUsage,
		},
		{
			name: "help flag",
			args: []string{"firstarg", "secondarg"},
			help: true,
			err:  ErrUsage,
		},
		{
			name: "first file failed to open",
			args: []string{"firstarg", "secondarg"},
			err:  os.ErrNotExist,
		},
		{
			name: "second file failed to open",
			args: []string{filepath.Join(tmpdir, "file1"), "secondarg"},
			err:  os.ErrNotExist,
		},
		{
			name:  "comm case s1 > s2",
			args:  []string{filepath.Join(tmpdir, "file1"), filepath.Join(tmpdir, "file2")},
			file1: "line1\nline2\nline3\n",
			file2: "line1\nline2\nline4\n",
			want:  "\t\tline1\n\t\tline2\nline3\n\tline4\n",
		},
		{
			name:  "comm case s1 < s2",
			args:  []string{filepath.Join(tmpdir, "file1"), filepath.Join(tmpdir, "file2")},
			file1: "line1\nline2\nline4\n",
			file2: "line1\nline2\nline3\n",
			want:  "\t\tline1\n\t\tline2\n\tline3\nline4\n",
		},
		{
			name:  "comm flag s1 true",
			args:  []string{filepath.Join(tmpdir, "file1"), filepath.Join(tmpdir, "file2")},
			file1: "line1\nline2\nline4\n",
			file2: "line1\nline2\nline3\n",
			s1:    true,
			want:  "\t\tline1\n\t\tline2\n\tline3\n",
		},
		{
			name:  "comm flag s2 true",
			args:  []string{filepath.Join(tmpdir, "file1"), filepath.Join(tmpdir, "file2")},
			file1: "line1\nline2\nline4\n",
			file2: "line1\nline2\nline3\n",
			s2:    true,
			want:  "\t\tline1\n\t\tline2\nline4\n",
		},
		{
			name:  "comm flag s3 true",
			args:  []string{filepath.Join(tmpdir, "file1"), filepath.Join(tmpdir, "file2")},
			file1: "line1\nline2\nline4\n",
			file2: "line1\nline2\nline3\n",
			s3:    true,
			want:  "\tline3\nline4\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(tmpdir, "file1"), []byte(tt.file1), 0o644); err != nil {
				t.Errorf("failed to create file1: %v", err)
			}
			if err := os.WriteFile(filepath.Join(tmpdir, "file2"), []byte(tt.file2), 0o644); err != nil {
				t.Errorf("failed to create file1: %v", err)
			}

			buf := &bytes.Buffer{}
			if err := comm(buf, tt.s1, tt.s2, tt.s3, tt.help, tt.args...); !errors.Is(err, tt.err) {
				t.Errorf("comm() = %q, want: %q", err, tt.err)
			}
			if buf.String() != tt.want {
				t.Errorf("comm() = %q, want: %q", buf.String(), tt.want)
			}
		})
	}
}

func TestMainReadsStdinAndLongLines(t *testing.T) {
	dir := t.TempDir()
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(second, []byte("alpha\nomega\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Main(context.Background(), core.IO{
		Stdin:  strings.NewReader("alpha\nmiddle\n"),
		Stdout: &stdout,
		Stderr: &stderr,
		Dir:    dir,
	}, "-", "second.txt")
	if code != 0 {
		t.Fatalf("comm exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "\t\talpha\nmiddle\n\tomega\n"; got != want {
		t.Errorf("comm output = %q, want %q", got, want)
	}

	longLine := strings.Repeat("x", 128*1024) + "\r"
	lines, err := readLines(context.Background(), strings.NewReader(longLine))
	if err != nil {
		t.Fatalf("readLines returned error: %v", err)
	}
	if len(lines) != 1 || lines[0] != longLine {
		t.Errorf("readLines did not preserve long final line and carriage return")
	}
}

func TestMainPropagatesOutputError(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code := Main(context.Background(), core.IO{
		Stdin:  strings.NewReader(""),
		Stdout: commErrorWriter{err: errors.New("write failed")},
		Stderr: io.Discard,
	}, first, second)
	if code == 0 {
		t.Fatal("comm returned success after output failure")
	}
}

type commErrorWriter struct {
	err error
}

func (w commErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
