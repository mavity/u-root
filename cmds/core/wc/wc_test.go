// Copyright 2016-2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestFiles(t *testing.T) {
	tmpDir := t.TempDir()
	f1, err := os.CreateTemp(tmpDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer f1.Close()
	f2, err := os.CreateTemp(tmpDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	_, err = f1.Write([]byte("simple test count words\nlines\nlines\n"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		want string
		err  error
		args []string
		p    params
	}{
		{
			name: "count in 2 files",
			args: []string{f1.Name(), f2.Name()},
			want: "3 6 36 " + f1.Name() + "\n0 0 0 " + f2.Name() + "\n3 6 36 total\n",
		},
		{
			name: "file does not exist",
			args: []string{"filedoesnotexist"},
			err:  os.ErrNotExist,
		},
		{
			name: "both files do not exist but total printed",
			args: []string{"filedoesnotexist1", "filedoesnotexist2"},
			err:  os.ErrNotExist,
			want: "0 0 0 total\n",
		},
		{
			name: "file1 no flag",
			args: []string{f1.Name()},
			want: fmt.Sprintf("3 6 36 %s\n", f1.Name()),
		},
		{
			name: "count words in file1",
			args: []string{f1.Name()},
			p:    params{words: true},
			want: fmt.Sprintf("6 %s\n", f1.Name()),
		},
		{
			name: "count lines in file1",
			args: []string{f1.Name()},
			p:    params{lines: true},
			want: fmt.Sprintf("3 %s\n", f1.Name()),
		},
		{
			name: "count lines in file1",
			args: []string{f1.Name()},
			p:    params{chars: true},
			want: fmt.Sprintf("36 %s\n", f1.Name()),
		},
		{
			name: "count runes in file1",
			args: []string{f1.Name()},
			p:    params{runes: true},
			want: fmt.Sprintf("36 %s\n", f1.Name()),
		},
		{
			name: "count broken in file1",
			args: []string{f1.Name()},
			p:    params{broken: true},
			want: fmt.Sprintf("0 %s\n", f1.Name()),
		},
		{
			name: "count in 2 files",
			args: []string{f1.Name(), f2.Name()},
			want: fmt.Sprintf("3 6 36 %s\n0 0 0 %s\n3 6 36 total\n", f1.Name(), f2.Name()),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := io.Discard
			err := command(nil, stdout, stderr, test.p, test.args).run()
			if !errors.Is(err, test.err) {
				t.Fatalf("expected %v, got %v", test.err, err)
			}
			if stdout.String() != test.want {
				t.Errorf("wc stdout = %q, want: %q", stdout.String(), test.want)
			}
		})
	}
}

func TestStdin(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		args  []string
		p     params
	}{
		{
			name:  "ascii",
			input: "hello world\n",
			want:  "1 2 12\n",
		},
		{
			name:  "utf8",
			input: "München\n",
			want:  "1 1 9\n",
		},
		{
			name:  "empty",
			input: "",
			want:  "0 0 0\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdin := bytes.NewBufferString(test.input)
			stdout := &bytes.Buffer{}

			if err := command(stdin, stdout, nil, test.p, test.args).run(); err != nil {
				t.Fatal(err)
			}

			if got := stdout.String(); got != test.want {
				t.Errorf("wc %q = %q, want: %q", test.input, got, test.want)
			}
		})
	}
}

func TestMainCharacterAndLongestLineFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(context.Background(), core.IO{
		Stdin:  bytes.NewBufferString("hello\nMünchen\nlast"),
		Stdout: &stdout,
		Stderr: &stderr,
	}, "-m", "-L")
	if code != 0 {
		t.Fatalf("wc exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "18 7\n"; got != want {
		t.Errorf("wc output = %q, want %q", got, want)
	}
}

func TestMainLongestLineDisplayWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "ascii", input: "hello\n", want: "5\n"},
		{name: "wide character", input: "\u754C\n", want: "2\n"},
		{name: "tab stop", input: "a\tb\n", want: "9\n"},
		{name: "tab after column eight", input: "12345678\tx\n", want: "17\n"},
		{name: "combining grapheme", input: "e\u0301\n", want: "1\n"},
		{name: "emoji zwj grapheme", input: "\U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466\n", want: "2\n"},
		{name: "ansi styling", input: "\x1b[31m\u754C\x1b[0m\n", want: "2\n"},
		{name: "crlf", input: "\u754C\r\n", want: "2\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Main(context.Background(), core.IO{
				Stdin:  bytes.NewBufferString(test.input),
				Stdout: &stdout,
				Stderr: &stderr,
			}, "-L")
			if code != 0 {
				t.Fatalf("wc exit code = %d, want 0; stderr = %q", code, stderr.String())
			}
			if got := stdout.String(); got != test.want {
				t.Errorf("wc -L output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMainPropagatesOutputError(t *testing.T) {
	code := Main(context.Background(), core.IO{
		Stdin:  bytes.NewBufferString("line\n"),
		Stdout: wcErrorWriter{err: errors.New("write failed")},
		Stderr: io.Discard,
	}, "-l")
	if code == 0 {
		t.Fatal("wc returned success after output failure")
	}
}

type wcErrorWriter struct {
	err error
}

func (w wcErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
