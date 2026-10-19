// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package more

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

func TestMore(t *testing.T) {
	t.Run("files is not exist", func(t *testing.T) {
		err := run(nil, nil, 40, []string{"file-is-not-exists"})
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %v, got %v", os.ErrNotExist, err)
		}
	})
	t.Run("negative lines", func(t *testing.T) {
		err := run(nil, nil, -1, []string{"file1"})
		if !errors.Is(err, errLinesMustBePositive) {
			t.Errorf("expected %v, got %v", errLinesMustBePositive, err)
		}
	})
	t.Run("one screen file", func(t *testing.T) {
		content := "line1\nline2\nline3\n"
		path := filepath.Join(t.TempDir(), "file1")
		err := os.WriteFile(path, []byte(content), 0o644)
		if err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		stdout := &bytes.Buffer{}

		err = run(nil, stdout, 10, []string{path})
		if err != nil {
			t.Fatalf("failed to run more: %v", err)
		}

		if stdout.String() != content {
			t.Errorf("expected %q, got %q", content, stdout.String())
		}
	})
	t.Run("two screen file", func(t *testing.T) {
		content := "line1\nline2\nline3\nline4\nline5\n"
		// third '\n' missing, 'enter' is emulated by stdin buffer
		// '\n' comes from 'enter'
		expectedOutput := "line1\nline2\nline3line4\nline5\n"
		path := filepath.Join(t.TempDir(), "file1")
		err := os.WriteFile(path, []byte(content), 0o644)
		if err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		stdin := bytes.NewBufferString("a")
		stdout := &bytes.Buffer{}

		if err := run(stdin, stdout, 3, []string{path}); err != nil {
			t.Fatalf("failed to run more: %v", err)
		}

		if stdout.String() != expectedOutput {
			t.Errorf("expected %q, got %q", expectedOutput, stdout.String())
		}
	})
}

func TestMainInvocation(t *testing.T) {
	ctx := context.Background()

	t.Run("stdin non-interactive paging", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader("alpha\nbeta\ngamma\n"), Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "--lines=5")
		if code != 0 {
			t.Fatalf("Main returned %d, want 0; stderr=%s", code, stderr.String())
		}
		if stdout.String() != "alpha\nbeta\ngamma\n" {
			t.Errorf("stdout = %q, want alpha\\nbeta\\ngamma\\n", stdout.String())
		}
	})

	t.Run("file with relative path resolution", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "doc.txt")
		if err := os.WriteFile(p, []byte("line1\nline2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Dir: dir, Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "doc.txt")
		if code != 0 {
			t.Fatalf("Main returned %d, want 0; stderr=%s", code, stderr.String())
		}
		if stdout.String() != "line1\nline2\n" {
			t.Errorf("stdout = %q, want line1\\nline2\\n", stdout.String())
		}
	})

	t.Run("non-positive lines flag returns 1", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "--lines=-5")
		if code != 1 {
			t.Fatalf("Main returned %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "lines must be positive") {
			t.Errorf("stderr %q should contain lines must be positive", stderr.String())
		}
	})

	t.Run("help flag returns 0", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "-help")
		if code != 0 {
			t.Fatalf("Main with -help returned %d, want 0", code)
		}
	})
}
