// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/u-root/u-root/pkg/core"
)

func setupTestFiles(t *testing.T, data []byte) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	var names []string
	for i, b := range data {
		name := fmt.Sprintf("file%d", i)
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte{b}, 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return dir, names
}

func TestCat_Files(t *testing.T) {
	someData := []byte{'l', 2, 3, 4, 'd'}
	dir, files := setupTestFiles(t, someData)

	var out, errOut bytes.Buffer
	ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: dir}
	code := Main(context.Background(), ioEnv, files...)
	if code != 0 {
		t.Fatalf("Main returned %d, stderr: %s", code, errOut.String())
	}
	if !bytes.Equal(out.Bytes(), someData) {
		t.Fatalf("got %v, want %v", out.Bytes(), someData)
	}
}

func TestCat_StdinPipe(t *testing.T) {
	testString := "testdata\n"
	var out, errOut bytes.Buffer
	ioEnv := core.IO{
		Stdin:  strings.NewReader(testString),
		Stdout: &out,
		Stderr: &errOut,
	}
	code := Main(context.Background(), ioEnv)
	if code != 0 {
		t.Fatalf("Main returned %d, stderr: %s", code, errOut.String())
	}
	if out.String() != testString {
		t.Fatalf("got %q, want %q", out.String(), testString)
	}
}

func TestCat_FileNotFound(t *testing.T) {
	var out, errOut bytes.Buffer
	ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
	code := Main(context.Background(), ioEnv, "nonexistent-file-xyz.txt")
	if code != 1 {
		t.Fatalf("expected code 1 for non-existent file, got %d", code)
	}
	if !strings.Contains(errOut.String(), "nonexistent-file-xyz.txt") {
		t.Fatalf("expected error message in stderr, got %q", errOut.String())
	}
}

func TestCat_DashInterleaved(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "f1.txt")
	f2 := filepath.Join(dir, "f2.txt")
	_ = os.WriteFile(f1, []byte("line1\nline2\n"), 0o644)
	_ = os.WriteFile(f2, []byte("line4\nline5\n"), 0o644)

	var out, errOut bytes.Buffer
	ioEnv := core.IO{
		Stdin:  strings.NewReader("line3\n"),
		Stdout: &out,
		Stderr: &errOut,
		Dir:    dir,
	}
	code := Main(context.Background(), ioEnv, "f1.txt", "-", "f2.txt")
	if code != 0 {
		t.Fatalf("Main returned %d, stderr: %s", code, errOut.String())
	}
	want := "line1\nline2\nline3\nline4\nline5\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

func TestCat_IOError(t *testing.T) {
	var out, errOut bytes.Buffer
	ioEnv := core.IO{
		Stdin:  iotest.ErrReader(errors.New("simulated read error")),
		Stdout: &out,
		Stderr: &errOut,
	}
	code := Main(context.Background(), ioEnv)
	if code != 1 {
		t.Fatalf("expected code 1 on read error, got %d", code)
	}
	if !strings.Contains(errOut.String(), "error concatenating to stdout") {
		t.Fatalf("expected error message in stderr, got %q", errOut.String())
	}
}

func TestCat_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out, errOut bytes.Buffer
	ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
	code := Main(ctx, ioEnv, "somefile.txt")
	if code != 130 {
		t.Fatalf("expected code 130 for cancelled context, got %d", code)
	}
}

func TestCat_FlagsAndHelp(t *testing.T) {
	// -h should return 0
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-h")
		if code != 0 {
			t.Fatalf("expected code 0 for -h, got %d", code)
		}
		if !strings.Contains(errOut.String(), "Usage:") {
			t.Fatalf("expected usage message in stderr, got: %q", errOut.String())
		}
	}

	// -u is an accepted unbuffered flag
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{
			Stdin:  strings.NewReader("unbuffered text\n"),
			Stdout: &out,
			Stderr: &errOut,
		}
		code := Main(context.Background(), ioEnv, "-u")
		if code != 0 || out.String() != "unbuffered text\n" {
			t.Fatalf("cat -u failed: code=%d out=%q err=%s", code, out.String(), errOut.String())
		}
	}

	// invalid flag should return 1
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-invalid-flag-xyz")
		if code != 1 {
			t.Fatalf("expected code 1 for invalid flag, got %d", code)
		}
	}
}

func TestCat_ReentrancyAndConcurrency(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 50; i++ {
		p := filepath.Join(dir, fmt.Sprintf("file_%d.txt", i))
		if err := os.WriteFile(p, []byte(fmt.Sprintf("content_%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const goroutines = 50
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			var out, errOut bytes.Buffer
			ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: dir}
			code := Main(context.Background(), ioEnv, fmt.Sprintf("file_%d.txt", id))
			if code != 0 {
				errCh <- fmt.Errorf("goroutine %d exited with code %d: %s", id, code, errOut.String())
				return
			}
			expected := fmt.Sprintf("content_%d", id)
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

func TestCat_ZeroPanic(t *testing.T) {
	// Calling with uninitialized or empty core.IO should not panic
	var errOut bytes.Buffer
	ioEnv := core.IO{Stderr: &errOut}
	code := Main(context.Background(), ioEnv)
	if code != 1 {
		t.Fatalf("expected code 1 for nil stdin, got %d", code)
	}
}

