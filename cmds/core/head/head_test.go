// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package head

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestMain(t *testing.T) {
	dir := t.TempDir()
	content := "first\nsecond\nthird\nfourth\nfifth\nsixth\nseventh\neighth\nninth\ntenth\neleventh\ntwelfth\n"
	if err := os.WriteFile(filepath.Join(dir, "lines.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		args     []string
		stdin    string
		want     string
		wantCode int
	}{
		{name: "default line count", args: []string{"lines.txt"}, want: "first\nsecond\nthird\nfourth\nfifth\nsixth\nseventh\neighth\nninth\ntenth\n"},
		{name: "requested line count", args: []string{"-n", "2", "lines.txt"}, want: "first\nsecond\n"},
		{name: "zero lines", args: []string{"-n", "0", "lines.txt"}, want: ""},
		{name: "requested byte count", args: []string{"-c", "8", "lines.txt"}, want: "first\nse"},
		{name: "zero bytes", args: []string{"-c", "0", "lines.txt"}, want: ""},
		{name: "stdin lines", args: []string{"-n", "1"}, stdin: "hello\nagain\n", want: "hello\n"},
		{name: "stdin bytes", args: []string{"-c", "5"}, stdin: "hello\nagain\n", want: "hello"},
		{name: "stdin file operand", args: []string{"-n", "1", "-"}, stdin: "hello\nagain\n", want: "hello\n"},
		{name: "multiple files", args: []string{"-n", "1", "lines.txt", "other.txt"}, want: "==> lines.txt <==\nfirst\n\n==> other.txt <==\nother\n"},
		{name: "cannot combine modes", args: []string{"-c", "1", "-n", "1"}, wantCode: 2},
		{name: "negative count", args: []string{"-n", "-1"}, wantCode: 2},
		{name: "missing file", args: []string{"missing.txt"}, wantCode: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			ioEnv := core.IO{
				Stdin:  strings.NewReader(tt.stdin),
				Stdout: &stdout,
				Stderr: &stderr,
				Dir:    dir,
			}
			if code := Main(context.Background(), ioEnv, tt.args...); code != tt.wantCode {
				t.Fatalf("Main() exit code = %d, want %d; stderr: %q", code, tt.wantCode, stderr.String())
			}
			if stdout.String() != tt.want {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.want)
			}
			if tt.name == "missing file" && !strings.Contains(stderr.String(), "missing.txt") {
				t.Errorf("stderr = %q, want missing filename", stderr.String())
			}
		})
	}
}

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestMainReadError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	errRead := errors.New("read failed")
	ioEnv := core.IO{Stdin: failingReader{errRead}, Stdout: &stdout, Stderr: &stderr}
	if code := Main(context.Background(), ioEnv, "-c", "1"); code != 1 {
		t.Fatalf("Main() exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "read failed") {
		t.Errorf("stderr = %q, want read error", stderr.String())
	}
}

func TestMainCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{Stdin: strings.NewReader("hello\n"), Stdout: &stdout, Stderr: &stderr}
	if code := Main(ctx, ioEnv, "-n", "1"); code != 130 {
		t.Fatalf("Main() exit code = %d, want 130", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestCopyLinesLongLine(t *testing.T) {
	line := strings.Repeat("a", bufioDefaultMaxTokenSize+1)
	var output bytes.Buffer
	if err := copyLines(context.Background(), strings.NewReader(line), &output, 1); err != nil {
		t.Fatal(err)
	}
	if output.String() != line {
		t.Errorf("copied %d bytes, want %d", output.Len(), len(line))
	}
}

const bufioDefaultMaxTokenSize = 64 * 1024
