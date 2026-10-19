// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lscmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/ls"
)

// Test listName func
func TestListName(t *testing.T) {
	// Create some directories.
	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, "d1"), 0777); err != nil {
		t.Fatalf("err in os.Mkdir: %v", err)
	}
	// Create some files.
	hasNewlineFile := runtime.GOOS != "windows"
	files := []string{"f1", "f2", ".f4", "d1/f4"}
	if hasNewlineFile {
		files = append(files, "f3\nline 2")
	}
	for _, file := range files {
		f, err := os.Create(filepath.Join(tmpDir, file))
		if err != nil {
			t.Fatalf("err in os.Create: %v", err)
		}
		_ = f.Close()
	}

	nlEntry := func(entry string) string {
		if hasNewlineFile {
			return entry + "\n"
		}
		return ""
	}

	// Creating test table
	for _, tt := range []struct {
		name   string
		input  string
		want   string
		flag   cmd
		prefix bool
	}{
		{
			name:  "ls without arguments",
			input: tmpDir,
			want:  fmt.Sprintf("d1\nf1\nf2\n%s", nlEntry("f3?line 2")),
			flag:  cmd{},
		},
		{
			name:  "ls osfi.IsDir() path, quoted = true",
			input: tmpDir,
			want:  fmt.Sprintf("\"d1\"\n\"f1\"\n\"f2\"\n%s", nlEntry("\"f3\\nline 2\"")),
			flag: cmd{
				quoted: true,
			},
		},
		{
			name:  "ls osfi.IsDir() path, quoted = true, prefix = true ",
			input: tmpDir,
			want:  fmt.Sprintf("%q:\n\"d1\"\n\"f1\"\n\"f2\"\n%s", tmpDir, nlEntry("\"f3\\nline 2\"")),
			flag: cmd{
				quoted: true,
			},
			prefix: true,
		},
		{
			name:   "ls osfi.IsDir() path, quoted = false, prefix = true ",
			input:  tmpDir,
			want:   fmt.Sprintf("%s:\nd1\nf1\nf2\n%s", tmpDir, nlEntry("f3?line 2")),
			prefix: true,
		},
		{
			name:  "ls recurse = true",
			input: tmpDir,
			want: func() string {
				var b strings.Builder
				b.WriteString(tmpDir + "\n")
				b.WriteString(filepath.Join(tmpDir, ".f4") + "\n")
				b.WriteString(filepath.Join(tmpDir, "d1") + "\n")
				b.WriteString(filepath.Join(tmpDir, "d1", "f4") + "\n")
				b.WriteString(filepath.Join(tmpDir, "f1") + "\n")
				b.WriteString(filepath.Join(tmpDir, "f2") + "\n")
				if hasNewlineFile {
					b.WriteString(filepath.Join(tmpDir, "f3?line 2") + "\n")
				}
				return b.String()
			}(),
			flag: cmd{
				all:     true,
				recurse: true,
			},
		},
		{
			name:  "ls directory = true",
			input: tmpDir,
			want:  fmt.Sprintf("%s\n", filepath.Base(tmpDir)),
			flag: cmd{
				directory: true,
			},
		},
		{
			name:  "ls classify = true",
			input: tmpDir,
			want:  fmt.Sprintf("d1/\nf1\nf2\n%s", nlEntry("f3?line 2")),
			flag: cmd{
				classify: true,
			},
		},
		{
			name:  "file does not exist",
			input: "dir",
			want:  "lstat dir: no such file or directory\n",
			flag:  cmd{},
		},
	} {
		// Running the tests
		t.Run(tt.name, func(t *testing.T) {
			// Write output in buffer.
			var buf bytes.Buffer
			var s ls.Stringer = ls.NameStringer{}
			if tt.flag.quoted {
				s = ls.QuotedStringer{}
			}
			if tt.flag.long {
				s = ls.LongStringer{Human: tt.flag.human, Name: s}
			}
			tt.flag.w = &buf

			// listName always return nil
			_ = tt.flag.listName(s, tt.input, tt.prefix)
			if tt.name == "file does not exist" {
				if !strings.Contains(buf.String(), "no such file or directory") &&
					!strings.Contains(buf.String(), "The system cannot find the file specified") {
					t.Errorf("listName() = '%v', want error containing 'no such file or directory'", buf.String())
				}
				return
			}
			if buf.String() != tt.want {
				t.Errorf("listName() = '%v', want: '%v'", buf.String(), tt.want)
			}
		})
	}
}

// Test list func
func TestRun(t *testing.T) {
	// Creating test table
	for _, tt := range []struct {
		name   string
		args   []string
		err    error
		prefix bool
	}{
		{
			name: "input empty, quoted = true, long = true",
			args: []string{"ls", "-Ql"},
			err:  nil,
		},
		{
			name: "input empty, quoted = true, long = true",
			args: []string{"ls", "-Ql", "dir"},
			err:  os.ErrNotExist,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := run(io.Discard, tt.args); err != nil {
				if !errors.Is(err, tt.err) {
					t.Errorf("list() = '%v', want: '%v'", err, tt.err)
				}
			}
		})
	}
}

// Test indicator func
func TestIndicator(t *testing.T) {
	// Creating test table
	for _, test := range []struct {
		lsInfo ls.FileInfo
		symbol string
	}{
		{
			ls.FileInfo{
				Mode: os.ModeDir,
			},
			"/",
		},
		{
			ls.FileInfo{
				Mode: os.ModeNamedPipe,
			},
			"|",
		},
		{
			ls.FileInfo{
				Mode: os.ModeSymlink,
			},
			"@",
		},
		{
			ls.FileInfo{
				Mode: os.ModeSocket,
			},
			"=",
		},
		{
			ls.FileInfo{
				Mode: 0b110110100,
			},
			"",
		},
		{
			ls.FileInfo{
				Mode: 0b111111101,
			},
			"*",
		},
	} {
		// Run tests
		got := indicator(test.lsInfo)
		if got != test.symbol {
			t.Errorf("for mode '%b' expected '%q', got '%q'", test.lsInfo.Mode, test.symbol, got)
		}
	}
}

// Make sure if perms fail in a dir, we still list the dir.
func TestPermHandling(t *testing.T) {
	d := t.TempDir()
	for _, v := range []string{"a", "c", "d"} {
		if err := os.Mkdir(filepath.Join(d, v), 0777); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(d, "b"), 0); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"0", "1", "2"} {
		if err := os.Mkdir(filepath.Join(d, v), 0777); err != nil {
			t.Fatal(err)
		}
	}
	b := &bytes.Buffer{}
	var c = cmd{w: b}

	if err := c.listName(ls.NameStringer{}, d, false); err != nil {
		t.Fatalf("listName(ls.NameString{}, %q, w, false): %v != nil", d, err)
	}
	if !strings.Contains(b.String(), "0\n1\n2\na\nb\nc\nd\n") {
		t.Errorf("ls %q: output %q did not contain %q", d, b.String(), "0\n1\n2\na\nb\nc\nd\n")
	}
}

func TestNotExist(t *testing.T) {
	d := t.TempDir()
	b := &bytes.Buffer{}
	var c = cmd{w: b}
	if err := c.listName(ls.NameStringer{}, filepath.Join(d, "b"), false); err != nil {
		t.Fatalf("listName(ls.NameString{}, %q/b, w, false): nil != %v", d, err)
	}
	eexist := fmt.Sprintf("%s:%v", filepath.Join(d, "b"), os.ErrNotExist)
	enoent := fmt.Sprintf("%s: %v", filepath.Join(d, "b"), syscall.ENOENT)
	if !strings.Contains(b.String(), eexist) && !strings.Contains(b.String(), enoent) &&
		!strings.Contains(b.String(), "cannot find the file specified") &&
		!strings.Contains(b.String(), "no such file or directory") {
		t.Fatalf("ls of bad name: %q does not contain %q or %q", b.String(), eexist, enoent)
	}
}

func TestMainInvocation(t *testing.T) {
	ctx := context.Background()

	t.Run("basic ls in directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("b"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Dir: dir, Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv)
		if code != 0 {
			t.Fatalf("Main returned %d, want 0; stderr=%s", code, stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "alpha.txt") || !strings.Contains(out, "beta.txt") {
			t.Errorf("stdout = %q, expected alpha.txt and beta.txt", out)
		}
	})

	t.Run("ls non-existent returns 1", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "definitely-not-existing-path-12345")
		if code != 1 {
			t.Fatalf("expected code 1 on non-existent file, got %d", code)
		}
	})

	t.Run("help flag returns 0", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(ctx, ioEnv, "--help")
		if code != 0 {
			t.Fatalf("expected code 0 on --help, got %d", code)
		}
	})
}
