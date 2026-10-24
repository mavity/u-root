// Copyright 2016-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package chmod

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestOmittedWhoUsesUmaskAndReportsMaskedRemoval(t *testing.T) {
	oldUmask := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(oldUmask) })

	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, nil, 0o444); err != nil {
		t.Fatal(err)
	}
	ioEnv := core.IO{Stdout: io.Discard, Stderr: io.Discard}

	if err := run(context.Background(), ioEnv, "+w", path); err != nil {
		t.Fatalf("run +w: %v", err)
	}
	assertMode(t, path, 0o644)

	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), ioEnv, "a+w", path); err != nil {
		t.Fatalf("run a+w: %v", err)
	}
	assertMode(t, path, 0o666)

	err := run(context.Background(), ioEnv, "-w", path)
	if err == nil || !strings.Contains(err.Error(), "new permissions are r--rw-rw-, not r--r--r--") {
		t.Fatalf("run -w error = %v, want GNU masked-removal diagnostic", err)
	}
	assertMode(t, path, 0o466)
}

func TestRecursiveSymlinkBehavior(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	child := filepath.Join(tree, "child")
	childFile := filepath.Join(child, "file")
	outside := filepath.Join(dir, "outside")
	outsideFile := filepath.Join(outside, "file")
	rootLink := filepath.Join(dir, "root-link")
	nestedLink := filepath.Join(tree, "nested-link")
	nestedFileLink := filepath.Join(tree, "nested-file-link")
	for _, path := range []string{tree, child, outside} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(childFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(tree, rootLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, nestedLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, nestedFileLink); err != nil {
		t.Fatal(err)
	}

	ioEnv := core.IO{Stdout: io.Discard, Stderr: io.Discard}
	if err := run(context.Background(), ioEnv, "-R", "0700", tree); err != nil {
		t.Fatalf("recursive chmod of tree: %v", err)
	}
	assertMode(t, tree, 0o700)
	assertMode(t, child, 0o700)
	assertMode(t, childFile, 0o700)
	assertMode(t, outside, 0o755)
	assertMode(t, outsideFile, 0o644)

	for path, mode := range map[string]uint16{
		tree: 0o755, child: 0o755, outside: 0o755, outsideFile: 0o644,
	} {
		if err := os.Chmod(path, posixModeToFileMode(mode)); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(context.Background(), ioEnv, "-R", "0711", rootLink); err != nil {
		t.Fatalf("recursive chmod of command-line symlink: %v", err)
	}
	assertMode(t, tree, 0o711)
	assertMode(t, child, 0o711)
	assertMode(t, childFile, 0o711)
	assertMode(t, outside, 0o755)
	assertMode(t, outsideFile, 0o644)

	fileLink := filepath.Join(dir, "file-link")
	targetFile := filepath.Join(dir, "target-file")
	if err := os.WriteFile(targetFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetFile, fileLink); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), ioEnv, "0600", fileLink); err != nil {
		t.Fatalf("chmod command-line symlink to file: %v", err)
	}
	assertMode(t, targetFile, 0o600)
}

func TestMainEmitsMultipleOperandErrorsOnce(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	ioEnv := core.IO{Stdout: io.Discard, Stderr: &stderr}

	code := Main(context.Background(), ioEnv, "0600", file, filepath.Join(dir, "missing"))
	if code == 0 {
		t.Fatal("Main returned success with a missing operand")
	}
	if !errors.Is(run(context.Background(), ioEnv, "0600", filepath.Join(dir, "missing")), os.ErrNotExist) {
		t.Fatal("missing path error does not wrap os.ErrNotExist")
	}
	if count := strings.Count(stderr.String(), "no such file or directory"); count != 1 {
		t.Fatalf("error diagnostic count = %d, output %q", count, stderr.String())
	}
	assertMode(t, file, 0o600)
}
