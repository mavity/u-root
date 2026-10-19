// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package readlink

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

type test struct {
	args      []string
	out       string
	stdErr    string
	hasError  bool
	follow    bool
	noNewLine bool
	verbose   bool
}

func TestReadlink(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "readLinkDir")
	if err := os.Mkdir(testDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Create files
	f1Path := filepath.Join(testDir, "f1")
	f1, err := os.Create(f1Path)
	if err != nil {
		t.Fatal(err)
	}
	_ = f1.Close()

	f2Path := filepath.Join(testDir, "f2")
	f2, err := os.Create(f2Path)
	if err != nil {
		t.Fatal(err)
	}
	_ = f2.Close()

	// Create symlinks
	f1Symlink := filepath.Join(testDir, "f1symlink")
	if err := os.Symlink("f1", f1Symlink); err != nil {
		t.Fatal(err)
	}

	multiLinks := filepath.Join(testDir, "multilinks")
	if err := os.Symlink(f1Symlink, multiLinks); err != nil {
		t.Fatal(err)
	}

	tests := []test{
		{
			args:     []string{},
			out:      "",
			stdErr:   "",
			hasError: true,
		},
		{
			args:     []string{},
			out:      "",
			stdErr:   "missing operand",
			hasError: true,
			verbose:  true,
		},
		{
			args:     []string{"f1"},
			out:      "",
			stdErr:   "readlink",
			hasError: true,
			verbose:  true,
		},
		{
			args:     []string{"f2"},
			out:      "",
			stdErr:   "",
			hasError: true,
			follow:   true,
		},
		{
			args:     []string{"f1symlink"},
			out:      "f1\n",
			stdErr:   "",
			hasError: false,
		},
		{
			args:      []string{"f1symlink"},
			out:       "f1",
			stdErr:    "",
			hasError:  false,
			noNewLine: true,
		},
		{
			args:     []string{"multilinks"},
			out:      f1Symlink + "\n",
			stdErr:   "",
			hasError: false,
		},
		{
			args:     []string{"multilinks", "f1symlink", "f2"},
			out:      f1Symlink + "\nf1\n",
			stdErr:   "readlink",
			hasError: true,
			verbose:  true,
		},
		{
			args:     []string{testDir},
			out:      "",
			stdErr:   "readlink",
			hasError: true,
			verbose:  true,
		},
		{
			args:     []string{"foo.bar"},
			out:      "",
			stdErr:   "readlink",
			hasError: true,
			verbose:  true,
		},
	}

	for i, tt := range tests {
		var out, stdErr bytes.Buffer
		ioEnv := core.IO{
			Stdout: &out,
			Stderr: &stdErr,
			Dir:    testDir,
		}

		var cmdArgs []string
		if tt.verbose {
			cmdArgs = append(cmdArgs, "-v")
		}
		if tt.noNewLine {
			cmdArgs = append(cmdArgs, "-n")
		}
		if tt.follow {
			cmdArgs = append(cmdArgs, "-f")
		}
		cmdArgs = append(cmdArgs, tt.args...)

		err := run(context.Background(), ioEnv, cmdArgs...)

		outNorm := strings.ReplaceAll(out.String(), "\\", "/")
		expectedNorm := strings.ReplaceAll(tt.out, "\\", "/")
		if outNorm != expectedNorm {
			t.Errorf("test %d stdout got:\n%s\nwant:\n%s", i, outNorm, expectedNorm)
		}

		if tt.stdErr != "" && !strings.Contains(stdErr.String(), tt.stdErr) {
			t.Errorf("test %d stderr got:\n%s\nwant containing:\n%s", i, stdErr.String(), tt.stdErr)
		}

		if tt.hasError && err == nil {
			t.Errorf("test %d expected to exit with error", i)
		}

		if !tt.hasError && err != nil {
			t.Errorf("test %d expected to exit without error: %v", i, err)
		}
	}
}
