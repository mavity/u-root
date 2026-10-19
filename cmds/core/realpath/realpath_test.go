// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package realpath

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestRealpath(t *testing.T) {
	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.CreateTemp(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	err = os.Symlink(f.Name(), filepath.Join(dir, "symlink"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		expectedErr    error
		expectedOutput string
		args           []string
	}{
		{
			expectedOutput: f.Name() + "\n",
			args:           []string{f.Name()},
		},
		{
			expectedOutput: f.Name() + "\n",
			args:           []string{filepath.Join(dir, "symlink")},
		},
		{
			expectedOutput: f.Name() + "\n",
			args:           []string{dir + "/../" + filepath.Base(dir) + "/symlink"},
		},
		{
			args:        []string{"filenotexists"},
			expectedErr: os.ErrNotExist,
		},
	}

	for _, test := range tests {
		stdout := &bytes.Buffer{}
		ioEnv := core.IO{Stdout: stdout, Stderr: io.Discard, Dir: dir}

		err := run(context.Background(), ioEnv, test.args...)
		if !errors.Is(err, test.expectedErr) {
			t.Fatalf("args %v: expected %v, got %v", test.args, test.expectedErr, err)
		}

		outNorm := strings.ReplaceAll(stdout.String(), "\\", "/")
		expectedNorm := strings.ReplaceAll(test.expectedOutput, "\\", "/")
		if outNorm != expectedNorm {
			t.Errorf("args %v: expected %v, got %v", test.args, expectedNorm, outNorm)
		}
	}
}
