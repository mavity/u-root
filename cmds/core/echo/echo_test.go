// Copyright 2015-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package echo

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

// Test echo cmd
func TestEcho(t *testing.T) {
	// Creating test table for test
	for _, tt := range []struct {
		name                      string
		input                     string
		want                      string
		noNewline                 bool
		interpretEscapes          bool
		interpretBackslashEscapes bool
	}{
		{
			name:      "test1 noNewline",
			input:     "simple test1",
			want:      "simple test1",
			noNewline: true,
		},
		{
			name:  "test2",
			input: "simple test2",
			want:  "simple test2\n",
		},
		{
			name:             "test3 interpretEscapes",
			input:            "simple\\ttest3",
			want:             "simple\ttest3\n",
			interpretEscapes: true,
		},
		{
			name:             "test4 interpretEscapes",
			input:            "simple\\ttest4",
			want:             "simple\ttest4\n",
			interpretEscapes: true,
		},
		{
			name:             "test5 interpretEscapes",
			input:            "simple\\tte\\cst5",
			want:             "simple\tte\n",
			interpretEscapes: true,
		},
		{
			name:             "test6 interpretEscapes and noNewline",
			input:            "simple\\tte\\cst6",
			want:             "simple\tte",
			noNewline:        true,
			interpretEscapes: true,
		},
		{
			name:             "test7 interpretEscapes and noNewline",
			input:            "simple\\x56 test7",
			want:             "simpleV test7",
			noNewline:        true,
			interpretEscapes: true,
		},
		{
			name:             "test8 interpretEscapes and noNewline",
			input:            "simple\\x56 \\0113test8",
			want:             "simpleV Ktest8",
			noNewline:        true,
			interpretEscapes: true,
		},
		{
			name:             "test9 interpretEscapes and noNewline",
			input:            "\\\\9",
			want:             "\\9",
			noNewline:        true,
			interpretEscapes: true,
		},
		{
			name:             "test10 empty String",
			input:            "",
			want:             "",
			noNewline:        true,
			interpretEscapes: true,
		},
		{
			name:                      "test11 interpretBackslashEscapes true",
			input:                     "simple test11",
			want:                      "simple test11\n",
			interpretBackslashEscapes: true,
		},
	} {
		// Run tests
		t.Run(tt.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := echo(&got, tt.noNewline, tt.interpretEscapes, tt.interpretBackslashEscapes, tt.input); err != nil {
				t.Errorf("%q failed: %q", tt.name, err)
			}
			if got.String() != tt.want {
				t.Fatalf("%q failed. Got: %q, Want: %q", tt.name, got.String(), tt.want)
			}
		})
	}
}

func TestEchoMain(t *testing.T) {
	var out, errOut bytes.Buffer
	ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
	code := Main(t.Context(), ioEnv, "-n", "hello", "world")
	if code != 0 {
		t.Fatalf("Main returned %d, err: %s", code, errOut.String())
	}
	if out.String() != "hello world" {
		t.Fatalf("got %q, want 'hello world'", out.String())
	}
}

func TestEchoQuotesAndBackslashC(t *testing.T) {
	var out, errOut bytes.Buffer
	ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
	code := Main(t.Context(), ioEnv, "-e", "hello \"world\"")
	if code != 0 {
		t.Fatalf("Main returned %d, err: %s", code, errOut.String())
	}
	if out.String() != "hello \"world\"\n" {
		t.Fatalf("got %q, want 'hello \"world\"\\n'", out.String())
	}
}

func TestEchoReentrancyAndConcurrency(t *testing.T) {
	const goroutines = 50
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			var out, errOut bytes.Buffer
			ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
			code := Main(context.Background(), ioEnv, "-n", fmt.Sprintf("id=%d", id))
			if code != 0 {
				errCh <- fmt.Errorf("goroutine %d exited with code %d: %s", id, code, errOut.String())
				return
			}
			expected := fmt.Sprintf("id=%d", id)
			if out.String() != expected {
				errCh <- fmt.Errorf("goroutine %d got %q, want %q", id, out.String(), expected)
				return
			}
			errCh <- nil
		}(i)
	}
	for i := 0; i < goroutines; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
}

func TestEchoContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	var out, errOut bytes.Buffer
	ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
	code := Main(ctx, ioEnv, "should", "not", "run")
	if code != 130 {
		t.Fatalf("expected code 130 for cancelled context, got %d", code)
	}
}

func TestEchoFlagsAndHelp(t *testing.T) {
	// -h should return 0
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-h")
		if code != 0 {
			t.Fatalf("expected code 0 for -h, got %d", code)
		}
		if !strings.Contains(errOut.String(), "Usage:") {
			t.Fatalf("expected usage message in stderr, got: %q", errOut.String())
		}
	}

	// invalid flag should return 1
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-invalid-flag-xyz")
		if code != 1 {
			t.Fatalf("expected code 1 for invalid flag, got %d", code)
		}
	}
}

func TestEchoEscapesFull(t *testing.T) {
	// Test -E disables escapes
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-E", "hello\\nworld")
		if code != 0 || out.String() != "hello\\nworld\n" {
			t.Fatalf("-E failed: got %q, code=%d", out.String(), code)
		}
	}

	// Test \0NNN octal and \xHH hex
	{
		var out, errOut bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: &errOut}
		code := Main(context.Background(), ioEnv, "-n", "-e", "\\0101\\x42")
		if code != 0 || out.String() != "AB" {
			t.Fatalf("octal/hex failed: got %q, code=%d", out.String(), code)
		}
	}
}


