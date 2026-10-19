// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sync

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestSync(t *testing.T) {
	d := t.TempDir()
	file1, err := os.CreateTemp(d, "file1")
	if err != nil {
		t.Fatalf("failed to create tmp file1: %v", err)
	}
	defer file1.Close()

	file2, err := os.CreateTemp(d, "file2")
	if err != nil {
		t.Fatalf("failed to create tmp file2: %v", err)
	}
	defer file2.Close()

	for _, tt := range []struct {
		name       string
		input      []string
		wantErr    bool
		errContain string
		data       bool
		filesystem bool
	}{
		{
			name:  "data flag",
			input: []string{file1.Name(), file2.Name()},
			data:  true,
		},
		{
			name:       "data flag with wrong path",
			input:      []string{"nonexistent_file_xyz"},
			wantErr:    true,
			errContain: "nonexistent_file_xyz",
			data:       true,
		},
		{
			name:       "filesystem flag",
			input:      []string{file1.Name(), file2.Name()},
			filesystem: true,
		},
		{
			name:  "default",
			input: []string{file1.Name(), file2.Name()},
		},
		{
			name:  "no args",
			input: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := sync(tt.data, tt.filesystem, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("sync() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Fatalf("sync() = %v, want to contain %q", err, tt.errContain)
				}
			} else {
				if err != nil {
					t.Fatalf("sync() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestMain(t *testing.T) {
	d := t.TempDir()
	f, err := os.CreateTemp(d, "synctest")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}

	code := Main(context.Background(), ioEnv, f.Name())
	if code != 0 {
		t.Fatalf("Main(%s) returned %d, stderr: %s", f.Name(), code, stderr.String())
	}

	code = Main(context.Background(), ioEnv, "-help")
	if code != 0 {
		t.Fatalf("Main(-help) returned %d, stderr: %s", code, stderr.String())
	}
}
