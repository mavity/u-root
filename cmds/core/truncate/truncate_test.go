// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package truncate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rck/unit"
	"github.com/u-root/u-root/pkg/core"
)

func TestTruncate(t *testing.T) {
	tmpdir := t.TempDir()
	for _, tt := range []struct {
		name     string
		args     []string
		create   bool
		ioBlocks bool
		size     unit.Value
		sizeWant int64
		rfile    string
		want     string
	}{
		{
			name: "!size.IsSet && *rfile == \"\"",
			want: "you need to specify size via -s <number> or -r <rfile>",
		},
		{
			name: "size.IsSet && *rfile == \"\"",
			size: unit.Value{
				IsSet: true,
			},
			rfile: "testfile",
			want:  "you need to specify size via -s <number> or -r <rfile>",
		},
		{
			name:  "len(args) == 0",
			rfile: "testfile",
			want:  "you need to specify one or more files as argument",
		},
		{
			name:   "create non existing file with err in rfile path",
			create: false,
			rfile:  "testfile",
			args:   []string{filepath.Join(tmpdir, "file1")},
			want:   "could not stat reference file:",
		},
		{
			name:   "create non existing file without err in rfile path",
			create: false,
			rfile:  filepath.Join(tmpdir, "file1"),
			args:   []string{filepath.Join(tmpdir, "file1")},
		},
		{
			name:   "truncate existing file without err and positive size",
			create: false,
			size: unit.Value{
				Value:        -50,
				IsSet:        true,
				ExplicitSign: unit.Sign(2),
			},
			sizeWant: 0,
			args:     []string{filepath.Join(tmpdir, "file1")},
		},
		{
			name:   "truncate existing file without err and negative size",
			create: false,
			size: unit.Value{
				Value:        50,
				IsSet:        true,
				ExplicitSign: unit.Sign(1),
			},
			sizeWant: 50,
			args:     []string{filepath.Join(tmpdir, "file1")},
		},
		{
			name:   "truncate with io blocks flag",
			create: false,
			size: unit.Value{
				Value: 2,
				IsSet: true,
			},
			ioBlocks: true,
			sizeWant: 1024,
			args:     []string{filepath.Join(tmpdir, "file1")},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var cmdArgs []string
			if tt.create {
				cmdArgs = append(cmdArgs, "-c")
			}
			if tt.ioBlocks {
				cmdArgs = append(cmdArgs, "-o")
			}
			if tt.size.IsSet {
				sign := ""
				if tt.size.ExplicitSign == unit.Sign(1) {
					sign = "+"
				} else if tt.size.ExplicitSign == unit.Sign(2) {
					sign = "-"
				}
				cmdArgs = append(cmdArgs, "-s", fmt.Sprintf("%s%d", sign, tt.size.Value))
			}
			if tt.rfile != "" {
				cmdArgs = append(cmdArgs, "-r", tt.rfile)
			}
			cmdArgs = append(cmdArgs, tt.args...)

			ioEnv := core.IO{Stdout: io.Discard, Stderr: io.Discard}
			err := run(context.Background(), ioEnv, cmdArgs...)
			if err != nil {
				if tt.want == "" || !strings.Contains(err.Error(), tt.want) {
					t.Errorf("truncate() = %q, want containing: %q", err.Error(), tt.want)
				}
			} else {
				if tt.want != "" {
					t.Errorf("truncate() expected error containing %q, got nil", tt.want)
				}
				if len(tt.args) > 0 {
					st, err := os.Stat(filepath.Join(tmpdir, "file1"))
					if err != nil {
						t.Errorf("failed to get file stats: %v", err)
					} else if st.Size() != tt.sizeWant {
						t.Errorf("the file was not truncated right, got file size %v, want: %v", st.Size(), tt.sizeWant)
					}
				}

			}
		})
	}
}

func TestTruncateReportsErrorsAndHonorsNoCreate(t *testing.T) {
	tmpdir := t.TempDir()
	ioEnv := core.IO{Stdout: io.Discard, Stderr: io.Discard}

	dir := filepath.Join(tmpdir, "directory")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), ioEnv, "-s", "1", dir); err == nil {
		t.Fatal("run() succeeded when asked to truncate a directory")
	}

	missing := filepath.Join(tmpdir, "missing")
	if err := run(context.Background(), ioEnv, "-c", "-s", "1", missing); err != nil {
		t.Fatalf("run() with -c on a missing file returned error: %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("run() with -c created the missing file, stat error = %v", err)
	}
}
