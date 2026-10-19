// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package md5sum

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestMd5Sum(t *testing.T) {
	// Creating tmp files with data to hash
	tmpdir := t.TempDir()
	file1Path := filepath.Join(tmpdir, "file1")
	if err := os.WriteFile(file1Path, []byte("abcdef\n"), 0o644); err != nil {
		t.Fatalf("failed to write string to file1: %v", err)
	}

	file2Path := filepath.Join(tmpdir, "file2")
	if err := os.WriteFile(file2Path, []byte("pqra\n"), 0o644); err != nil {
		t.Fatalf("failed to write string to file2: %v", err)
	}

	for _, tt := range []struct {
		name string
		args []string
		want string
		err  bool
	}{
		{
			name: "bufIn as input",
			args: []string{},
			want: "8d777f385d3dfec8815d20f7496026dc -\n",
		},
		{
			name: "wrong path file",
			args: []string{"testfile"},
			err:  true,
		},
		{
			name: "file1 as input",
			args: []string{file1Path},
			want: fmt.Sprintf("%s %s\n", "5ab557c937e38f15291c04b7e99544ad", file1Path),
		},
		{
			name: "file2 as input",
			args: []string{file2Path},
			want: fmt.Sprintf("%s %s\n", "721d6b135656aa83baca6ebdbd2f6c86", file2Path),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bufIn := bytes.NewBufferString("data")
			bufOut := &bytes.Buffer{}
			ioEnv := core.IO{Stdin: bufIn, Stdout: bufOut, Stderr: io.Discard}

			err := run(context.Background(), ioEnv, tt.args...)
			if tt.err {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if bufOut.String() != tt.want {
					t.Errorf("md5Sum() = %q, want: %q", bufOut.String(), tt.want)
				}
			}
		})
	}

	// Test check mode (-c)
	t.Run("check mode", func(t *testing.T) {
		checkFile := filepath.Join(tmpdir, "checksums.md5")
		content := fmt.Sprintf("5ab557c937e38f15291c04b7e99544ad  %s\n", file1Path)
		if err := os.WriteFile(checkFile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: io.Discard}
		if err := run(context.Background(), ioEnv, "-c", checkFile); err != nil {
			t.Fatalf("check failed: %v", err)
		}
		if !strings.Contains(stdout.String(), "OK") {
			t.Errorf("expected OK in check output, got %q", stdout.String())
		}
	})
}
