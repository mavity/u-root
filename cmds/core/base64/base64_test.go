// Copyright 2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package base64cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

type failer struct{}

// Write implements io.Writer, and always fails with os.ErrInvalid
func (failer) Write([]byte) (int, error) {
	return -1, os.ErrInvalid
}

func TestBase64(t *testing.T) {
	tests := []struct {
		in  []byte
		out []byte
	}{
		{
			in: []byte(`DESCRIPTION
       Base64 encode or decode FILE, or standard input, to standard output.

       With no FILE, or when FILE is -, read standard input.

       Mandatory arguments to long options are mandatory for short options too.
`),
			out: []byte(`REVTQ1JJUFRJT04KICAgICAgIEJhc2U2NCBlbmNvZGUgb3IgZGVjb2RlIEZJTEUsIG9yIHN0YW5kYXJkIGlucHV0LCB0byBzdGFuZGFyZCBvdXRwdXQuCgogICAgICAgV2l0aCBubyBGSUxFLCBvciB3aGVuIEZJTEUgaXMgLSwgcmVhZCBzdGFuZGFyZCBpbnB1dC4KCiAgICAgICBNYW5kYXRvcnkgYXJndW1lbnRzIHRvIGxvbmcgb3B0aW9ucyBhcmUgbWFuZGF0b3J5IGZvciBzaG9ydCBvcHRpb25zIHRvby4K
`),
		},
	}
	d := t.TempDir()
	for _, tt := range tests {
		nin := filepath.Join(d, "in")
		if err := os.WriteFile(nin, tt.in, 0o666); err != nil {
			t.Fatalf(`WriteFile(%q, %v, 0666): %v != nil`, nin, tt.in, err)
		}
		nout := filepath.Join(d, "out")
		if err := os.WriteFile(nout, tt.out, 0o666); err != nil {
			t.Fatalf(`WriteFile(%q, %v, 0666): %v != nil`, nout, tt.out, err)
		}

		// Loop over encodes, then loop over decodes
		for _, n := range [][]string{{nin}, {}} {
			t.Run(fmt.Sprintf("run with file name %q", n), func(t *testing.T) {
				var o bytes.Buffer
				err := run(nil, &o, io.Discard, nin)
				if err != nil {
					t.Errorf("encode: got %v, want nil", err)
					return
				}
				if !bytes.Equal(o.Bytes(), tt.out) {
					t.Errorf("encode: %q != %q", o.Bytes(), tt.out)
				}
			})
		}

		for _, n := range [][]string{{nout}, {}} {
			t.Run(fmt.Sprintf("run with file name %q", n), func(t *testing.T) {

				var o bytes.Buffer
				err := run(nil, &o, io.Discard, "-d", nout)
				if err != nil {
					t.Errorf("decode: got %v, want nil", err)
					return
				}
				if !bytes.Equal(o.Bytes(), tt.in) {
					t.Errorf("decode: %q != %q", o.Bytes(), tt.out)
				}
			})
		}
	}
	// Try opening a file we know does not exist.
	n := filepath.Join(d, "nosuchfile")
	t.Run(fmt.Sprintf("bad file %q", n), func(t *testing.T) {
		err := run(nil, nil, nil, n)
		if err == nil {
			t.Errorf("run(%q): nil != an error", n)
		}
	})

	// Try with a bad length
	t.Run("bad data", func(t *testing.T) {
		err := run(bytes.NewReader([]byte{'t'}), io.Discard, io.Discard, "-d")
		if err == nil {
			t.Errorf(`run("-d"): nil != an error`)
		}
	})
}

func TestBadWriter(t *testing.T) {
	err := run(bytes.NewReader([]byte("hi")), failer{}, io.Discard)
	if !errors.Is(err, os.ErrInvalid) {
		t.Errorf(`run(): got %v, want %v`, err, os.ErrInvalid)
	}
}

func TestBadUsage(t *testing.T) {
	tests := []struct {
		args []string
		err  error
	}{
		{args: []string{"x", "y"}, err: errBadUsage},
	}

	for _, tt := range tests {
		err := run(nil, io.Discard, io.Discard, tt.args...)
		if !errors.Is(err, tt.err) {
			t.Errorf(`Run(%q): got %v, want %v`, tt.args, err, tt.err)
		}
	}
}

func TestDo(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "single character",
			input: "a",
		},
		{
			name:  "four bytes",
			input: "abcd",
		},
		{
			name:  "five bytes",
			input: "abcde",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var encoded bytes.Buffer
			err := run(strings.NewReader(tt.input), &encoded, io.Discard)
			if err != nil {
				t.Fatalf("encoding: got %v, want nil", err)
			}

			var decoded bytes.Buffer
			err = run(&encoded, &decoded, io.Discard, []string{"-d"}...)
			if err != nil {
				t.Fatalf("decoding: got %v, want nil", err)
			}

			d := decoded.String()
			if d != tt.input {
				t.Errorf("encode/decode failed: got %q, want %q", d, tt.input)
			}
		})
	}
}

func TestMainInvocation(t *testing.T) {
	ctx := context.Background()

	t.Run("stdin encode", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader("hello"), Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv)
		if code != 0 {
			t.Fatalf("Main returned %d, want 0; stderr=%s", code, stderr.String())
		}
		if stdout.String() != "aGVsbG8=\n" {
			t.Errorf("stdout = %q, want %q", stdout.String(), "aGVsbG8=\n")
		}
	})

	t.Run("stdin decode", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdin: strings.NewReader("aGVsbG8=\n"), Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "-d")
		if code != 0 {
			t.Fatalf("Main returned %d, want 0; stderr=%s", code, stderr.String())
		}
		if stdout.String() != "hello" {
			t.Errorf("stdout = %q, want %q", stdout.String(), "hello")
		}
	})

	t.Run("file input with resolver", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "data.txt")
		if err := os.WriteFile(filePath, []byte("world"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Dir: dir, Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "data.txt")
		if code != 0 {
			t.Fatalf("Main returned %d, want 0; stderr=%s", code, stderr.String())
		}
		if stdout.String() != "d29ybGQ=\n" {
			t.Errorf("stdout = %q, want %q", stdout.String(), "d29ybGQ=\n")
		}
	})

	t.Run("help flag returns 0", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "-help")
		if code != 0 {
			t.Fatalf("Main with -help returned %d, want 0", code)
		}
	})

	t.Run("cancellation returns 130", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{
			Stdin:  strings.NewReader("input"),
			Stdout: &stdout,
			Stderr: &stderr,
		}
		if code := Main(cancelled, ioEnv); code != 130 {
			t.Fatalf("Main returned %d, want 130; stderr=%s", code, stderr.String())
		}
	})
}
