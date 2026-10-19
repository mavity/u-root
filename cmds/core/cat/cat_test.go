// Copyright 2012 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// by Rafael Campos Nunes <rafaelnunes@engineer.com>

package cat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/u-root/u-root/pkg/core"
)

// setup writes a set of files, putting 1 byte in each file.
func setup(t *testing.T, data []byte) string {
	t.Helper()
	t.Logf(":: Creating simulation data. ")
	dir := t.TempDir()

	for i, d := range data {
		n := fmt.Sprintf("%v%d", filepath.Join(dir, "file"), i)
		if err := os.WriteFile(n, []byte{d}, 0o666); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

// TestCat test cat function against 4 files, in each file it is written a bit of someData
// array and the test expect the cat to return the exact same bit from someData array with
// the corresponding file.
func TestCat(t *testing.T) {
	var files []string
	someData := []byte{'l', 2, 3, 4, 'd'}

	dir := setup(t, someData)

	for i := range someData {
		files = append(files, fmt.Sprintf("%v%d", filepath.Join(dir, "file"), i))
	}
	var out bytes.Buffer
	if err := run(nil, &out, files...); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(out.Bytes(), someData) {
		t.Fatalf("Reading files failed: got %v, want %v", out.Bytes(), someData)
	}
}

func TestCatPipe(t *testing.T) {
	var inputbuf bytes.Buffer
	teststring := "testdata"
	fmt.Fprintf(&inputbuf, "%s", teststring)

	var out bytes.Buffer

	if err := cat(&inputbuf, &out); err != nil {
		t.Error(err)
	}
	if out.String() != teststring {
		t.Errorf("CatPipe: Want %q Got: %q", teststring, out.String())
	}
}

func TestRunFiles(t *testing.T) {
	var files []string
	someData := []byte{'l', 2, 3, 4, 'd'}

	dir := setup(t, someData)

	for i := range someData {
		files = append(files, fmt.Sprintf("%v%d", filepath.Join(dir, "file"), i))
	}

	var out bytes.Buffer
	if err := run(nil, &out, files...); err != nil {
		t.Error(err)
	}
	if !bytes.Equal(out.Bytes(), someData) {
		t.Fatalf("Reading files failed: got %v, want %v", out.Bytes(), someData)
	}
}

func TestRunFilesError(t *testing.T) {
	var files []string
	someData := []byte{'l', 2, 3, 4, 'd'}

	dir := setup(t, someData)

	for i := range someData {
		files = append(files, fmt.Sprintf("%v%d", filepath.Join(dir, "file"), i))
	}
	filenotexist := "testdata/doesnotexist.txt"
	files = append(files, filenotexist)
	var in, out bytes.Buffer
	if err := run(&in, &out, files...); err == nil {
		t.Error("function run succeeded but should have failed")
	}
}

func TestRunNoArgs(t *testing.T) {
	var in, out bytes.Buffer
	inputdata := "teststring"
	fmt.Fprintf(&in, "%s", inputdata)
	if err := run(&in, &out); err != nil {
		t.Error(err)
	}
	if out.String() != inputdata {
		t.Errorf("Want: %q Got: %q", inputdata, out.String())
	}
}

func TestIOErrors(t *testing.T) {
	stdout := bytes.Buffer{}
	errReader := iotest.ErrReader(errors.New("read error"))
	err := run(errReader, &stdout)
	if !errors.Is(err, errCopy) {
		t.Errorf("expected %v, got %v", errCopy, err)
	}

	err = run(errReader, &stdout, "-")
	if !errors.Is(err, errCopy) {
		t.Errorf("expected %v, got %v", errCopy, err)
	}
}

func TestCatDash(t *testing.T) {
	tempDir := t.TempDir()

	f1 := path.Join(tempDir, "f1")
	err := os.WriteFile(f1, []byte("line1\nline2\n"), 0o666)
	if err != nil {
		t.Fatal(err)
	}

	f2 := path.Join(tempDir, "f2")
	err = os.WriteFile(f2, []byte("line4\nline5\n"), 0o666)
	if err != nil {
		t.Fatal(err)
	}

	var stdin, stdout bytes.Buffer
	stdin.WriteString("line3\n")

	if err = run(&stdin, &stdout, f1, "-", f2); err != nil {
		t.Fatal(err)
	}

	want := "line1\nline2\nline3\nline4\nline5\n"
	got := stdout.String()

	if got != want {
		t.Errorf("want: %s, got: %s", want, got)
	}
}

func TestCatMain(t *testing.T) {
	tempDir := t.TempDir()
	f := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(f, []byte("hello from cat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: tempDir}
	code := Main(t.Context(), ioEnv, "test.txt")
	if code != 0 {
		t.Fatalf("Main returned %d, err: %s", code, errOut.String())
	}
	if out.String() != "hello from cat\n" {
		t.Fatalf("got %q, want 'hello from cat\\n'", out.String())
	}
}

func TestCatReentrancyAndConcurrency(t *testing.T) {
	tempDir := t.TempDir()
	for i := 0; i < 50; i++ {
		p := filepath.Join(tempDir, fmt.Sprintf("file_%d.txt", i))
		if err := os.WriteFile(p, []byte(fmt.Sprintf("content_%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const goroutines = 50
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			var out, errOut bytes.Buffer
			ioEnv := core.IO{Stdout: &out, Stderr: &errOut, Dir: tempDir}
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

func TestCatContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out, errOut bytes.Buffer
	ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
	code := Main(ctx, ioEnv, "somefile.txt")
	if code != 130 {
		t.Fatalf("expected code 130 for cancelled context, got %d", code)
	}
}

func TestCatFlagsAndHelp(t *testing.T) {
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

	// -u is an accepted ignored flag
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{
			Stdin:  strings.NewReader("unbuffered"),
			Stdout: &out,
			Stderr: &errOut,
		}
		code := Main(context.Background(), ioEnv, "-u")
		if code != 0 || out.String() != "unbuffered" {
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

func TestCatStdinAndDash(t *testing.T) {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "f1.txt")
	f2 := filepath.Join(tempDir, "f2.txt")
	_ = os.WriteFile(f1, []byte("AAA\n"), 0o644)
	_ = os.WriteFile(f2, []byte("CCC\n"), 0o644)

	var out, errOut bytes.Buffer
	ioEnv := core.IO{
		Stdin:  strings.NewReader("BBB\n"),
		Stdout: &out,
		Stderr: &errOut,
		Dir:    tempDir,
	}
	code := Main(context.Background(), ioEnv, "f1.txt", "-", "f2.txt")
	if code != 0 {
		t.Fatalf("cat f1 - f2 failed: code=%d err=%s", code, errOut.String())
	}
	if out.String() != "AAA\nBBB\nCCC\n" {
		t.Fatalf("got %q, want 'AAA\\nBBB\\nCCC\\n'", out.String())
	}
}

