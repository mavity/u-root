// Copyright 2018 the u-root Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/u-root/u-root/pkg/core"
)

func TestMain(t *testing.T) {
	dir := t.TempDir()
	content := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve"
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
		{name: "default lines", args: []string{"lines.txt"}, want: "three\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve"},
		{name: "requested lines", args: []string{"-n", "2", "lines.txt"}, want: "eleven\ntwelve"},
		{name: "one line", args: []string{"-n", "1", "lines.txt"}, want: "twelve"},
		{name: "zero lines", args: []string{"-n", "0", "lines.txt"}, want: ""},
		{name: "requested bytes", args: []string{"-c", "5", "lines.txt"}, want: "welve"},
		{name: "zero bytes", args: []string{"-c", "0", "lines.txt"}, want: ""},
		{name: "stdin lines", args: []string{"-n", "2"}, stdin: "alpha\nbeta\ngamma\n", want: "beta\ngamma\n"},
		{name: "stdin bytes", args: []string{"-c", "5"}, stdin: "alpha\nbeta\n", want: "beta\n"},
		{name: "stdin file operand", args: []string{"-n", "1", "-"}, stdin: "alpha\nbeta\n", want: "beta\n"},
		{name: "multiple files", args: []string{"-n", "1", "lines.txt", "other.txt"}, want: "==> lines.txt <==\ntwelve\n==> other.txt <==\nother\n"},
		{name: "cannot combine counts", args: []string{"-c", "1", "-n", "1"}, wantCode: 2},
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

func TestTailLargeCounts(t *testing.T) {
	input := strings.Repeat("x", 40*1024) + "last"
	var byteOutput bytes.Buffer
	if err := tailBytes(context.Background(), strings.NewReader(input), &byteOutput, 8); err != nil {
		t.Fatal(err)
	}
	if got, want := byteOutput.String(), "xxxxlast"; got != want {
		t.Fatalf("tailBytes() = %q, want %q", got, want)
	}

	lines := strings.Builder{}
	for i := 0; i < 1100; i++ {
		fmt.Fprintf(&lines, "%d\n", i)
	}
	var lineOutput bytes.Buffer
	if err := tailLines(context.Background(), strings.NewReader(lines.String()), &lineOutput, 3); err != nil {
		t.Fatal(err)
	}
	if got, want := lineOutput.String(), "1097\n1098\n1099\n"; got != want {
		t.Fatalf("tailLines() = %q, want %q", got, want)
	}
}

func TestMainFollowAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "follow.txt")
	if err := os.WriteFile(path, []byte("first\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := make(chan string, 4)
	writer := writerFunc(func(p []byte) (int, error) {
		output <- string(bytes.Clone(p))
		return len(p), nil
	})
	var stderr bytes.Buffer
	ioEnv := core.IO{Stdout: writer, Stderr: &stderr}
	done := make(chan int, 1)
	go func() {
		done <- Main(ctx, ioEnv, "-f", "-n", "1", path)
	}()

	select {
	case got := <-output:
		if got != "second\n" {
			t.Fatalf("initial output = %q, want %q", got, "second\n")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tail did not emit initial output")
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("third\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-output:
		if got != "third\n" {
			t.Fatalf("follow output = %q, want %q", got, "third\n")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tail did not emit appended data")
	}

	cancel()
	select {
	case code := <-done:
		if code != 130 {
			t.Fatalf("Main() exit code = %d, want 130; stderr: %q", code, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tail did not stop after context cancellation")
	}
}

func TestMainBrokenPipe(t *testing.T) {
	ioEnv := core.IO{
		Stdin:  strings.NewReader("a\nb\n"),
		Stdout: writerFunc(func([]byte) (int, error) { return 0, io.ErrClosedPipe }),
		Stderr: io.Discard,
	}
	if code := Main(context.Background(), ioEnv, "-n", "1"); code != 0 {
		t.Fatalf("Main() exit code = %d, want 0 for closed downstream pipe", code)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestMainReadError(t *testing.T) {
	errRead := errors.New("read failed")
	var stderr bytes.Buffer
	ioEnv := core.IO{Stdin: failingReader{errRead}, Stdout: io.Discard, Stderr: &stderr}
	if code := Main(context.Background(), ioEnv, "-n", "1"); code != 1 {
		t.Fatalf("Main() exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), errRead.Error()) {
		t.Errorf("stderr = %q, want read error", stderr.String())
	}
}

func TestMainCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{Stdin: strings.NewReader("one\ntwo\n"), Stdout: &stdout, Stderr: &stderr}
	if code := Main(ctx, ioEnv, "-n", "1"); code != 130 {
		t.Fatalf("Main() exit code = %d, want 130", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}
