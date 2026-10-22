// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lscmd

import (
	"bytes"
	"context"
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
			name:  "all entries include hidden files",
			input: tmpDir,
			want: func() string {
				return fmt.Sprintf(".\n..\n.f4\nd1\nf1\nf2\n%s", nlEntry("f3?line 2"))
			}(),
			flag: cmd{
				all: true,
			},
		},
		{
			name:  "ls directory = true",
			input: tmpDir,
			want:  fmt.Sprintf("%s\n", tmpDir),
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
			err := run(io.Discard, tt.args)
			if (err == nil) != (tt.err == nil) {
				t.Errorf("run() error = %v, want error status %v", err, tt.err)
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

func TestVersionCompare(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{
		{a: "file2", b: "file10", want: -1},
		{a: "file10", b: "file2", want: 1},
		{a: "file02", b: "file2", want: 1},
		{a: "١", b: "٢", want: -1},
	} {
		if got := versionCompare(tt.a, tt.b); got != tt.want {
			t.Errorf("versionCompare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestColumnLayout(t *testing.T) {
	var vertical, across bytes.Buffer
	writeColumns(&vertical, []string{"a", "bb", "ccc"}, 8, false)
	writeColumns(&across, []string{"a", "bb", "ccc"}, 8, true)
	if got, want := vertical.String(), "a   ccc\nbb\n"; got != want {
		t.Errorf("vertical columns = %q, want %q", got, want)
	}
	if got, want := across.String(), "a    bb\nccc\n"; got != want {
		t.Errorf("across columns = %q, want %q", got, want)
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

func TestGNUListingOptions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "zdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"small":      "x",
		"large":      "longer",
		".hidden":    "h",
		"ignore.tmp": "i",
		"file10":     "10",
		"file2":      "2",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (string, string, int) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{
			Dir:    dir,
			Stdout: &stdout,
			Stderr: &stderr,
			LookupEnv: func(key string) (string, bool) {
				return "", false
			},
		}
		code := Main(context.Background(), ioEnv, args...)
		return stdout.String(), stderr.String(), code
	}

	t.Run("almost all and ignore", func(t *testing.T) {
		out, _, code := run("-A", "--ignore=*.tmp")
		if code != 0 || !strings.Contains(out, ".hidden\n") || strings.Contains(out, "ignore.tmp") || strings.Contains(out, "\n.\n") {
			t.Fatalf("ls -A --ignore=*.tmp: code=%d out=%q", code, out)
		}
	})
	t.Run("all includes dot entries", func(t *testing.T) {
		out, _, code := run("-a")
		if code != 0 || !strings.HasPrefix(out, ".\n..\n") {
			t.Fatalf("ls -a: code=%d out=%q", code, out)
		}
	})
	t.Run("hide patterns are overridden by all", func(t *testing.T) {
		out, _, code := run("-a", "--hide=*.tmp")
		if code != 0 || !strings.Contains(out, "ignore.tmp") {
			t.Fatalf("ls -a --hide=*.tmp: code=%d out=%q", code, out)
		}
	})
	t.Run("version sorting", func(t *testing.T) {
		out, _, code := run("-1", "--sort=version", "file10", "file2")
		if code != 0 || out != "file2\nfile10\n" {
			t.Fatalf("version sort: code=%d out=%q", code, out)
		}
	})
	t.Run("recursive directory headers", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(dir, "zdir", "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "zdir", "nested", "file"), []byte("n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _, code := run("-R", "zdir")
		if code != 0 || !strings.Contains(out, "zdir:\n") || !strings.Contains(out, "zdir"+string(filepath.Separator)+"nested:\n") {
			t.Fatalf("recursive listing: code=%d out=%q", code, out)
		}
		out, _, code = run("-R", "zdir", "zdir")
		if code != 0 || strings.Count(out, "zdir:\n") != 2 {
			t.Fatalf("repeated recursive operands: code=%d out=%q", code, out)
		}
	})
	t.Run("multiple directory headers have no leading blank line", func(t *testing.T) {
		out, _, code := run("zdir", "adir")
		if code != 0 || strings.HasPrefix(out, "\n") || strings.Count(out, ":\n") != 2 {
			t.Fatalf("multiple directories: code=%d out=%q", code, out)
		}
	})
	t.Run("size sort is within each directory", func(t *testing.T) {
		out, _, code := run("-S")
		if code != 0 || strings.Index(out, "large") < 0 || strings.Index(out, "large") > strings.Index(out, "file10") {
			t.Fatalf("size sort: code=%d out=%q", code, out)
		}
	})
	t.Run("group directories first", func(t *testing.T) {
		out, _, code := run("--group-directories-first")
		if code != 0 || strings.Index(out, "zdir") > strings.Index(out, "small") {
			t.Fatalf("group directories first: code=%d out=%q", code, out)
		}
	})
	t.Run("metadata formats", func(t *testing.T) {
		for _, args := range [][]string{{"-i", "small"}, {"-s", "small"}, {"-l", "small"}, {"-g", "small"}, {"-G", "small"}, {"-o", "small"}, {"-n", "small"}} {
			out, _, code := run(args...)
			if code != 0 || !strings.Contains(out, "small") {
				t.Errorf("ls %v: code=%d out=%q", args, code, out)
			}
		}
	})
}

func TestRichTerminalRendering(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdirectory"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "bb", "界"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(values map[string]string) core.LookupEnvFunc {
		return func(key string) (string, bool) {
			v, ok := values[key]
			return v, ok
		}
	}
	run := func(env map[string]string, terminal bool, width int, args ...string) (string, string, int) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{
			Dir:              dir,
			Stdout:           &stdout,
			Stderr:           &stderr,
			StdoutIsTerminal: terminal,
			TerminalWidth:    width,
			LookupEnv:        lookup(env),
		}
		code := Main(context.Background(), ioEnv, args...)
		return stdout.String(), stderr.String(), code
	}

	t.Run("automatic presentation stays off when redirected", func(t *testing.T) {
		out, _, code := run(nil, false, 0, "--icons=auto", "--hyperlink=auto")
		if code != 0 || strings.Contains(out, "\x1b]8;") || strings.Contains(out, "\x1b[") || strings.Contains(out, "📄") {
			t.Fatalf("redirected output has terminal decoration: code=%d out=%q", code, out)
		}
	})
	t.Run("columns honor width and wide glyphs", func(t *testing.T) {
		out, _, code := run(nil, true, 8)
		if code != 0 || !strings.Contains(out, "a") || !strings.Contains(out, "界") {
			t.Fatalf("terminal columns: code=%d out=%q", code, out)
		}
		if got := displayWidth("\x1b[31m界\x1b[0m"); got != 2 {
			t.Fatalf("displayWidth(ANSI + wide rune) = %d, want 2", got)
		}
	})
	t.Run("NO_COLOR suppresses automatic color including empty value", func(t *testing.T) {
		out, _, code := run(map[string]string{"NO_COLOR": ""}, true, 80)
		if code != 0 || strings.Contains(out, "\x1b[") {
			t.Fatalf("NO_COLOR output: code=%d out=%q", code, out)
		}
		out, _, code = run(map[string]string{"NO_COLOR": ""}, true, 80, "--color=always")
		if code != 0 || !strings.Contains(out, "\x1b[") {
			t.Fatalf("explicit color override: code=%d out=%q", code, out)
		}
	})
	t.Run("automatic terminal color and icon", func(t *testing.T) {
		out, _, code := run(nil, true, 80, "--icons=auto")
		if code != 0 || !strings.Contains(out, "\x1b[1;34m") || !strings.Contains(out, "📁") {
			t.Fatalf("automatic terminal decorations: code=%d out=%q", code, out)
		}
	})
	t.Run("explicit rich options emit OSC and icons", func(t *testing.T) {
		out, _, code := run(nil, false, 0, "--icons=always", "--hyperlink=always", ".")
		if code != 0 || !strings.Contains(out, "\x1b]8;;file://") || !strings.Contains(out, "📄") {
			t.Fatalf("explicit decorations: code=%d out=%q", code, out)
		}
	})
}
