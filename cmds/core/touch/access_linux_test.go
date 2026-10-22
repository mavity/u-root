// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package touch

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/u-root/u-root/pkg/core"
)

func TestAccess(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.CreateTemp(tmp, "touch_test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	accessDate, err := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if code := Main(context.Background(), core.IO{Stderr: io.Discard}, "-a", "-d", "2023-01-01T00:00:00Z", f.Name()); code != 0 {
		t.Fatalf("touch exit code = %d, want 0", code)
	}

	mfi, err := f.Stat()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	at, err := referenceAccessTime(mfi)
	if err != nil {
		t.Fatalf("reading access time: %v", err)
	}
	if at.UnixNano() != accessDate.UnixNano() {
		t.Errorf("expected access time %v, got %v", accessDate, at)
	}
}
