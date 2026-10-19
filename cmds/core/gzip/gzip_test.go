// Copyright 2012-2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gzipcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestGZIPSmoke(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	wantContent := []byte("test file's content\nsecond line")
	err := os.WriteFile(filePath, wantContent, 0o644)
	if err != nil {
		t.Fatalf("os.WriteFile(%v, %v, 0o644) = %v, want nil", filePath, string(wantContent), err)
	}

	ioEnv := core.IO{Stdout: io.Discard, Stderr: io.Discard, Dir: dir}

	err = run(context.Background(), ioEnv, "gzip", "-9", "-b", "128", "-p", "1", "file.txt")
	if err != nil {
		t.Fatalf("run(gzip) = %v, want nil", err)
	}

	err = run(context.Background(), ioEnv, "gzip", "-d", "file.txt.gz")
	if err != nil {
		t.Fatalf("run(gzip -d) = %v, want nil", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("os.ReadFile(%v) = %v, want nil", filePath, err)
	}

	if !bytes.Equal(content, wantContent) {
		t.Errorf("os.ReadFile(%v) = %v, want %v", filePath, string(content), string(wantContent))
	}
}
