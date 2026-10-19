// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package tarcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestTar(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file")
	content := "hello from tar"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tarPath := filepath.Join(tmpDir, "file.tar")

	// Create
	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{Dir: tmpDir, Stdout: &stdout, Stderr: &stderr}
	if err := run(context.Background(), ioEnv, "-cvf", "file.tar", "file"); err != nil {
		t.Fatalf("run create failed: %v, stderr: %s", err, stderr.String())
	}

	if _, err := os.Stat(tarPath); err != nil {
		t.Fatalf("archive file not found: %v", err)
	}

	// List
	stdout.Reset()
	stderr.Reset()
	if err := run(context.Background(), ioEnv, "-tvf", "file.tar"); err != nil {
		t.Fatalf("run list failed: %v, stderr: %s", err, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("file")) {
		t.Fatalf("expected stdout to contain 'file', got: %q", stdout.String())
	}

	// Remove original file
	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}

	// Extract
	stdout.Reset()
	stderr.Reset()
	if err := run(context.Background(), ioEnv, "-xvf", "file.tar", "."); err != nil {
		t.Fatalf("run extract failed: %v, stderr: %s", err, stderr.String())
	}

	b, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != content {
		t.Errorf("expected %q, got %q", content, string(b))
	}
}

func TestCommandErrors(t *testing.T) {
	var tests = []struct {
		err  error
		args []string
		p    params
	}{
		{
			err: errCreateAndExtract,
			p:   params{create: true, extract: true},
		},
		{
			err: errCreateAndList,
			p:   params{create: true, list: true},
		},
		{
			err: errExtractAndList,
			p:   params{extract: true, list: true},
		},
		{
			err:  errExtractArgsLen,
			p:    params{extract: true},
			args: []string{"1", "2"},
		},
		{
			err: errMissingMandatoryFlag,
		},
		{
			err:  errEmptyFile,
			p:    params{extract: true, file: ""},
			args: []string{"1"},
		},
	}

	for _, tt := range tests {
		err := validate(tt.p, tt.args)
		if err != tt.err {
			t.Errorf("expected %v, got %v", tt.err, err)
		}
	}
}

