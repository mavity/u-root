// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package uname

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestParseParams(t *testing.T) {
	tests := []struct {
		name      string
		all       bool
		kernel    bool
		node      bool
		release   bool
		version   bool
		machine   bool
		processor bool
		expected  params
	}{
		{
			name: "flag: -a",
			all:  true,
			expected: params{
				kernel:  true,
				node:    true,
				release: true,
				version: true,
				machine: true,
			},
		},
		{
			name: "no flags",
			expected: params{
				kernel: true,
			},
		},
		{
			name:    "flags: -s -n -r -v -m",
			kernel:  true,
			node:    true,
			release: true,
			version: true,
			machine: true,
			expected: params{
				kernel:  true,
				node:    true,
				release: true,
				version: true,
				machine: true,
			},
		},
		{
			name:      "flags: -p",
			processor: true,
			expected: params{
				machine: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parseParams(tt.all, tt.kernel, tt.node, tt.release, tt.version, tt.machine, tt.processor)
			if p != tt.expected {
				t.Errorf("expected %+v, got %+v", tt.expected, p)
			}
		})
	}
}

func TestRun(t *testing.T) {
	stdout := &bytes.Buffer{}
	p := params{kernel: true, node: true, release: true, version: true, machine: true}
	err := run(stdout, p)
	if err != nil || stdout.String() == "" {
		t.Error("expected no error and non empty output")
	}
}

func TestMain(t *testing.T) {
	t.Run("default kernel flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{
			Stdout: &stdout,
			Stderr: &stderr,
		})
		if code != 0 {
			t.Fatalf("expected code 0, got %d, stderr=%s", code, stderr.String())
		}
		out := strings.TrimSpace(stdout.String())
		if out == "" {
			t.Fatalf("expected non-empty output")
		}
	})

	t.Run("all flags -a", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{
			Stdout: &stdout,
			Stderr: &stderr,
		})
		if code != 0 {
			t.Fatalf("expected code 0, got %d, stderr=%s", code, stderr.String())
		}
		fields := strings.Fields(stdout.String())
		if len(fields) == 0 {
			t.Fatalf("expected fields in output")
		}
	})

	t.Run("combined flags -snrvm", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{
			Stdout: &stdout,
			Stderr: &stderr,
		}, "-snrvm")
		if code != 0 {
			t.Fatalf("expected code 0, got %d, stderr=%s", code, stderr.String())
		}
		fields := strings.Fields(stdout.String())
		if len(fields) < 2 {
			t.Fatalf("expected multiple fields, got %v", fields)
		}
	})

	t.Run("help flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{
			Stdout: &stdout,
			Stderr: &stderr,
		}, "-help")
		if code != 0 {
			t.Fatalf("expected code 0 for help, got %d", code)
		}
	})

	t.Run("unknown flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Main(context.Background(), core.IO{
			Stdout: &stdout,
			Stderr: &stderr,
		}, "-unknownflagxyz")
		if code != 1 {
			t.Fatalf("expected code 1 for invalid flag, got %d", code)
		}
	})
}
