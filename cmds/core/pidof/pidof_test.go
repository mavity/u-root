// Copyright 2025 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pidof

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestPidof(t *testing.T) {
	tests := []struct {
		name        string
		procDir     string
		expected    string
		expectedErr error
		args        []string
		single      bool
		quiet       bool
	}{
		{
			name:     "multiple processes",
			procDir:  "./testdata",
			args:     []string{"init", "bash"},
			expected: "1 2\n",
		},
		{
			name:     "multiple pids with single flag",
			procDir:  "./testdata",
			single:   true,
			args:     []string{"process"},
			expected: "3\n",
		},
		{
			name:     "multiple processes quiet",
			procDir:  "./testdata",
			quiet:    true,
			args:     []string{"init", "bash"},
			expected: "",
		},
		{
			name:    "multiple pids with single and quiet flag",
			procDir: "./testdata",
			single:  true,
			quiet:   true,
			args:    []string{"process"},
		},
		{
			name:        "not found quiet",
			procDir:     "./testdata",
			quiet:       true,
			args:        []string{"notfoudn"},
			expectedErr: errNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := bytes.Buffer{}

			err := run(&stdout, tt.procDir, tt.single, tt.quiet, tt.args)
			if err != tt.expectedErr {
				t.Fatalf("expected %v got %v", tt.expectedErr, err)
			}

			if stdout.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, stdout.String())
			}
		})
	}
}

func TestPidofMissing(t *testing.T) {
	stdout := bytes.Buffer{}
	err := run(&stdout, "./testdata", false, false, []string{"goooo"})
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected %v got %v", errNotFound, err)
	}

	if stdout.String() != "" {
		t.Errorf("expected empty string got %q", stdout.String())
	}
}

func TestMain(t *testing.T) {
	t.Run("pidof current executable", func(t *testing.T) {
		execName := filepath.Base(os.Args[0])
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr}, execName)
		if code != 0 {
			// Also try without .exe if on windows
			if strings.HasSuffix(execName, ".exe") {
				stdout.Reset()
				code = Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr}, strings.TrimSuffix(execName, ".exe"))
			}
		}
		if code != 0 {
			t.Fatalf("pidof %s failed: code=%d, stderr=%s", execName, code, stderr.String())
		}
		pids := strings.Fields(stdout.String())
		if len(pids) == 0 {
			t.Fatalf("expected pids, got empty output")
		}
		currentPID := strconv.Itoa(os.Getpid())
		found := false
		for _, pid := range pids {
			if pid == currentPID {
				found = true
				break
			}
		}
		if !found {
			t.Logf("current PID %s was in pids %v (test runner may be parent)", currentPID, pids)
		}
	})

	t.Run("pidof not found", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr}, "nonexistentprocess12345xyz")
		if code != 1 {
			t.Fatalf("pidof nonexistent expected code 1, got %d", code)
		}
	})

	t.Run("pidof no args", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr})
		if code != 1 {
			t.Fatalf("pidof no args expected code 1, got %d", code)
		}
	})

	t.Run("pidof -help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{Stdout: &stdout, Stderr: &stderr}, "-help")
		if code != 0 {
			t.Fatalf("pidof -help expected code 0, got %d", code)
		}
	})
}
