// Copyright 2016-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grep

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestStdinGrep(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantOut  string
		wantCode int
		args     []string
	}{
		{
			name:     "dot_matches_all",
			input:    "hix\n",
			wantOut:  "hix\n",
			wantCode: 0,
			args:     []string{"."},
		},
		{
			name:     "quiet_match",
			input:    "hix\n",
			wantOut:  "",
			wantCode: 0,
			args:     []string{"-q", "."},
		},
		{
			name:     "case_insensitive_match",
			input:    "hix\n",
			wantOut:  "hix\n",
			wantCode: 0,
			args:     []string{"-i", "hix"},
		},
		{
			name:     "case_insensitive_no_match",
			input:    "hix\n",
			wantOut:  "",
			wantCode: 1,
			args:     []string{"-i", "hox"},
		},
		{
			name:     "case_insensitive_uppercase_input",
			input:    "HiX\n",
			wantOut:  "HiX\n",
			wantCode: 0,
			args:     []string{"-i", "hix"},
		},
		{
			name:     "line_number",
			input:    "hix\n",
			wantOut:  "1:hix\n",
			wantCode: 0,
			args:     []string{"-n", "hix"},
		},
		{
			name:     "explicit_expr_flag",
			input:    "hix\n",
			wantOut:  "hix\n",
			wantCode: 0,
			args:     []string{"-e", "hix"},
		},
		{
			name:     "count",
			input:    "hix\n",
			wantOut:  "1\n",
			wantCode: 0,
			args:     []string{"-c", "hix"},
		},
		{
			name:     "quiet_no_match",
			input:    "hix\n",
			wantOut:  "",
			wantCode: 1,
			args:     []string{"-q", "hello"},
		},
		{
			name:     "invert_match",
			input:    "hix\nfoo\n",
			wantOut:  "foo\n",
			wantCode: 0,
			args:     []string{"-v", "hix"},
		},
		{
			name:     "fixed_strings",
			input:    "a: [a-z]{1,2}\n",
			wantOut:  "a: [a-z]{1,2}\n",
			wantCode: 0,
			args:     []string{"-F", "{1,2}"},
		},
		{
			name:     "fixed_strings_case_insensitive",
			input:    "a: [a-Z]{1,2}\n",
			wantOut:  "a: [a-Z]{1,2}\n",
			wantCode: 0,
			args:     []string{"-F", "-i", "[a-z]"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			ioEnv := core.IO{
				Stdin:  strings.NewReader(tc.input),
				Stdout: &out,
				Stderr: &errOut,
			}
			code := Main(context.Background(), ioEnv, tc.args...)
			if code != tc.wantCode {
				t.Errorf("code = %d, want %d (stderr: %s)", code, tc.wantCode, errOut.String())
			}
			if out.String() != tc.wantOut {
				t.Errorf("stdout = %q, want %q", out.String(), tc.wantOut)
			}
		})
	}
}

func TestFilesGrep(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "f1.txt")
	f2 := filepath.Join(tmpDir, "f2.txt")
	f3 := filepath.Join(tmpDir, "f3.txt")

	if err := os.WriteFile(f1, []byte("hix\nnix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("hix\nhello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f3, []byte("hix\ngoodbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Multi-file grep includes file names
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: tmpDir}
		code := Main(context.Background(), ioEnv, "hello", "f1.txt", "f2.txt")
		if code != 0 {
			t.Fatalf("expected code 0, got %d (err: %s)", code, errOut.String())
		}
		want := "f2.txt:hello\n"
		if out.String() != want {
			t.Fatalf("got %q, want %q", out.String(), want)
		}
	}

	// -l: list files with matches
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: tmpDir}
		code := Main(context.Background(), ioEnv, "-l", "nix", "f1.txt", "f2.txt")
		if code != 0 {
			t.Fatalf("expected code 0, got %d (err: %s)", code, errOut.String())
		}
		if !strings.Contains(out.String(), "f1.txt") || strings.Contains(out.String(), "f2.txt") {
			t.Fatalf("got %q, expected only f1.txt", out.String())
		}
	}

	// -r: recursive search
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: tmpDir}
		code := Main(context.Background(), ioEnv, "-r", "-h", "goodbye", ".")
		if code != 0 {
			t.Fatalf("expected code 0, got %d (err: %s)", code, errOut.String())
		}
		want := "goodbye\n"
		if out.String() != want {
			t.Fatalf("got %q, want %q", out.String(), want)
		}
	}
}

func TestGrepMain(t *testing.T) {
	var out, errOut bytes.Buffer
	ioEnv := core.IO{
		Stdin:  strings.NewReader("apple\nbanana\napricot\n"),
		Stdout: &out,
		Stderr: &errOut,
	}
	code := Main(t.Context(), ioEnv, "^a")
	if code != 0 {
		t.Fatalf("Main returned %d, err: %s", code, errOut.String())
	}
	want := "apple\napricot\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}

	// Test no match returns exit code 1
	out.Reset()
	errOut.Reset()
	ioEnv.Stdin = strings.NewReader("apple\nbanana\n")
	code = Main(t.Context(), ioEnv, "^z")
	if code != 1 {
		t.Fatalf("expected code 1 on no match, got %d", code)
	}
}

func TestGrepReentrancyAndConcurrency(t *testing.T) {
	const goroutines = 50
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			input := fmt.Sprintf("prefix_match_%d\nother_line\n", id)
			var out, errOut bytes.Buffer
			ioEnv := core.IO{
				Stdin:  strings.NewReader(input),
				Stdout: &out,
				Stderr: &errOut,
			}
			pattern := fmt.Sprintf("match_%d", id)
			code := Main(context.Background(), ioEnv, pattern)
			if code != 0 {
				errCh <- fmt.Errorf("goroutine %d exited with code %d: %s", id, code, errOut.String())
				return
			}
			expected := fmt.Sprintf("prefix_match_%d\n", id)
			if out.String() != expected {
				errCh <- fmt.Errorf("goroutine %d got %q, want %q", id, out.String(), expected)
				return
			}
			errCh <- nil
		}(i)
	}
	for i := 0; i < goroutines; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
}

func TestGrepContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out, errOut bytes.Buffer
	ioEnv := core.IO{
		Stdin:  strings.NewReader("some\nlines\n"),
		Stdout: &out,
		Stderr: &errOut,
	}
	code := Main(ctx, ioEnv, "lines")
	if code != 130 {
		t.Fatalf("expected code 130 for cancelled context, got %d", code)
	}
}

func TestGrepFlags(t *testing.T) {
	input := "Apple\nbanana\nAPRICOT\n"

	// -i: case insensitive
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-i", "ap")
		if code != 0 || out.String() != "Apple\nAPRICOT\n" {
			t.Fatalf("-i failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// -v: invert match
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-v", "banana")
		if code != 0 || out.String() != "Apple\nAPRICOT\n" {
			t.Fatalf("-v failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// -c: count
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-c", "banana")
		if code != 0 || out.String() != "1\n" {
			t.Fatalf("-c failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// -n: line numbers
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-n", "banana")
		if code != 0 || out.String() != "2:banana\n" {
			t.Fatalf("-n failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// -q: quiet (match found returns 0, no output)
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-q", "banana")
		if code != 0 || out.String() != "" {
			t.Fatalf("-q match failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// -q: quiet (no match returns 1, no output)
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-q", "notfound")
		if code != 1 || out.String() != "" {
			t.Fatalf("-q no match failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// -F: fixed string (dots are literal, not regex wildcard)
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader("a.b\naxb\n"), Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-F", "a.b")
		if code != 0 || out.String() != "a.b\n" {
			t.Fatalf("-F failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// --help: help (returns 0)
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "--help")
		if code != 0 || !strings.Contains(errOut.String(), "Usage:") {
			t.Fatalf("--help failed: code=%d err=%s", code, errOut.String())
		}
	}

	// -h: no-filename flag (suppress file name prefix)
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader("match line\n"), Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-h", "match")
		if code != 0 || out.String() != "match line\n" {
			t.Fatalf("-h failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// invalid flag returns 2
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-invalid-flag-xyz")
		if code != 2 {
			t.Fatalf("expected code 2 for invalid flag, got %d", code)
		}
	}
}

func TestGrepFilesAndRecursive(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	_ = os.Mkdir(subDir, 0o755)

	f1 := filepath.Join(tempDir, "file1.txt")
	f2 := filepath.Join(subDir, "file2.txt")
	_ = os.WriteFile(f1, []byte("target in root\nother line\n"), 0o644)
	_ = os.WriteFile(f2, []byte("target in sub\n"), 0o644)

	// Single file with relative path
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: tempDir}
		code := Main(context.Background(), ioEnv, "target", "file1.txt")
		if code != 0 || out.String() != "target in root\n" {
			t.Fatalf("single file failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// Recursive -r
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: tempDir}
		code := Main(context.Background(), ioEnv, "-r", "-h", "target", ".")
		if code != 0 || !strings.Contains(out.String(), "target in root") || !strings.Contains(out.String(), "target in sub") {
			t.Fatalf("recursive -r failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// -l: list files with matches
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: tempDir}
		code := Main(context.Background(), ioEnv, "-l", "target", "file1.txt")
		if code != 0 || !strings.Contains(out.String(), "file1.txt") {
			t.Fatalf("-l failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}
}

