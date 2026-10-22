// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hexdump

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

func TestHexdump(t *testing.T) {
	const testString = "abcdefghijklmnopqrstuvwxyz"
	// Creating file and write content into it for testing purposes
	d := t.TempDir()
	n := filepath.Join(d, "testfile")
	if err := os.WriteFile(n, []byte(testString), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name      string
		filenames []string
		readInput string
		want      string
		wantErr   error
	}{
		{
			name:      "hexdump from Stdin",
			filenames: []string{},
			readInput: testString,
			want: `00000000  61 62 63 64 65 66 67 68  69 6a 6b 6c 6d 6e 6f 70  |abcdefghijklmnop|
00000010  71 72 73 74 75 76 77 78  79 7a                    |qrstuvwxyz|
0000001a
`,
		},
		{
			name:      "hexdump from file",
			filenames: []string{filepath.Join(d, "testfile")},
			want: `00000000  61 62 63 64 65 66 67 68  69 6a 6b 6c 6d 6e 6f 70  |abcdefghijklmnop|
00000010  71 72 73 74 75 76 77 78  79 7a                    |qrstuvwxyz|
0000001a
`,
		},
		{
			name:      "hexdump from 2 files",
			filenames: []string{filepath.Join(d, "testfile"), filepath.Join(d, "testfile")},
			want: `00000000  61 62 63 64 65 66 67 68  69 6a 6b 6c 6d 6e 6f 70  |abcdefghijklmnop|
00000010  71 72 73 74 75 76 77 78  79 7a 61 62 63 64 65 66  |qrstuvwxyzabcdef|
00000020  67 68 69 6a 6b 6c 6d 6e  6f 70 71 72 73 74 75 76  |ghijklmnopqrstuv|
00000030  77 78 79 7a                                       |wxyz|
00000034
`,
		},
		{
			name:      "hexdump from file that does not exist",
			filenames: []string{"error"},
			wantErr:   fmt.Errorf("open %s: no such file or directory", "error"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readBuf := &bytes.Buffer{}
			writeBuf := &bytes.Buffer{}
			readBuf.WriteString(tt.readInput)
			if got := hexdump(tt.filenames, readBuf, writeBuf); got != nil {
				// Different Go compilers deliver
				// different errors: either syscall.ENOENT
				// or os.ErrNotExist.
				// Once that is fixed, we can use errors.Is
				if tt.wantErr == nil {
					t.Fatalf("hexdump() = '%v', want: nil", got)
				}
				if !strings.HasPrefix(got.Error(), tt.wantErr.Error()[:10]) {
					t.Fatalf("hexdump() = '%v', want: '%v'", got, tt.wantErr)
				}
			}

			if writeBuf.String() != tt.want {
				t.Errorf("Console output: %q, want: %q", writeBuf.String(), tt.want)
			}
		})
	}
}

func TestMainFormatsAndRepeatedLines(t *testing.T) {
	input := strings.Repeat("A", 32)
	defaultLine := "0000000 " + strings.Repeat("4141 ", 8) + "\n"
	canonicalLine := "00000000  41 41 41 41 41 41 41 41  41 41 41 41 41 41 41 41  |AAAAAAAAAAAAAAAA|\n"
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "default suppresses duplicate rows",
			want: defaultLine + "*\n0000020\n",
		},
		{
			name: "verbose default prints duplicate rows",
			args: []string{"-v"},
			want: defaultLine + "0000010 " + strings.Repeat("4141 ", 8) + "\n0000020\n",
		},
		{
			name: "canonical suppresses duplicate rows",
			args: []string{"-C"},
			want: canonicalLine + "*\n00000020\n",
		},
		{
			name: "verbose canonical prints duplicate rows",
			args: []string{"-C", "-v"},
			want: canonicalLine + "00000010  41 41 41 41 41 41 41 41  41 41 41 41 41 41 41 41  |AAAAAAAAAAAAAAAA|\n00000020\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Main(context.Background(), core.IO{
				Stdin:  strings.NewReader(input),
				Stdout: &stdout,
				Stderr: &stderr,
			}, tt.args...)
			if code != 0 {
				t.Fatalf("hexdump exit code = %d, want 0; stderr = %q", code, stderr.String())
			}
			if got := stdout.String(); got != tt.want {
				t.Errorf("hexdump output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMainSkipAndLimitAndWriterError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(context.Background(), core.IO{
		Stdin:  strings.NewReader("ABCDEF"),
		Stdout: &stdout,
		Stderr: &stderr,
	}, "-C", "-s", "1", "-n", "2", "-v")
	if code != 0 {
		t.Fatalf("hexdump exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "00000001") || !strings.Contains(got, "42 43") || strings.Contains(got, "44") {
		t.Errorf("hexdump skip/limit output = %q", got)
	}

	code = Main(context.Background(), core.IO{
		Stdin:  strings.NewReader("A"),
		Stdout: hexdumpErrorWriter{err: errors.New("write failed")},
		Stderr: io.Discard,
	})
	if code == 0 {
		t.Fatal("hexdump returned success after output failure")
	}
}

type hexdumpErrorWriter struct {
	err error
}

func (w hexdumpErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
