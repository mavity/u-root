// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grep

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

// GrepTest is a table-driven which spawns grep with a variety of options and inputs.
// We need to look at any output data, as well as exit status (errQuite) for things like the -q switch.
func TestStdinGrep(t *testing.T) {
	tests := []struct {
		input  string
		output string
		err    error
		p      params
		args   []string
	}{
		// BEWARE: the IO package seems to want this to be newline terminated.
		// If you just use hix with no newline the test will fail. Yuck.
		{
			input:  "hix\n",
			output: "hix\n",
			err:    nil,
			args:   []string{"."},
		},
		{
			input:  "hix\n",
			output: "",
			err:    nil,
			p:      params{quiet: true},
			args:   []string{"."},
		},
		{
			input:  "hix\n",
			output: "hix\n",
			err:    nil,
			p:      params{caseInsensitive: true},
			args:   []string{"hix"},
		},
		{
			input:  "hix\n",
			output: "",
			err:    nil,
			p:      params{caseInsensitive: true},
			args:   []string{"hox"},
		},
		{
			input:  "HiX\n",
			output: "HiX\n",
			err:    nil,
			p:      params{caseInsensitive: true},
			args:   []string{"hix"},
		},
		{
			input:  "hix\n",
			output: "1:hix\n",
			err:    nil,
			p:      params{number: true},
			args:   []string{"hix"},
		},
		{
			input:  "hix\n",
			output: "hix\n",
			err:    nil,
			p:      params{expr: "hix"},
		},
		{
			input:  "hix\n",
			output: "1\n",
			err:    nil,
			p:      params{count: true},
			args:   []string{"hix"},
		},
		{
			input:  "hix",
			output: "",
			err:    errQuiet,
			p:      params{quiet: true},
			args:   []string{"hello"},
		},
		// These tests don't make a lot of sense the way we're running it, but
		// hopefully it'll make codecov shut up.
		{
			input:  "hix\n",
			output: "hix\n",
			err:    nil,
			p:      params{headers: true},
			args:   []string{"hix"},
		},
		{
			input:  "hix\n",
			output: "hix\n",
			err:    nil,
			p:      params{recursive: true},
			args:   []string{"hix"},
		},
		{
			input:  "hix\nfoo\n",
			output: "foo\n",
			err:    nil,
			p:      params{invert: true},
			args:   []string{"hix"},
		},
		{
			input:  "hix\n",
			output: "\n",
			err:    nil,
			p:      params{noShowMatch: true},
			args:   []string{"hix"},
		}, // no filename, so it just prints a newline
		{
			input:  "a: [a-z]{1,2}\n",
			output: "a: [a-z]{1,2}\n",
			err:    nil,
			p:      params{fixed: true},
			args:   []string{"{1,2}"},
		},
		{
			input:  "a: [a-Z]{1,2}\n",
			output: "a: [a-Z]{1,2}\n",
			err:    nil,
			p:      params{fixed: true, caseInsensitive: true},
			args:   []string{"[A-z]"},
		},
		{
			input:  "a\nb\nc\n",
			output: "b\n",
			err:    nil,
			p:      params{fixed: true},
			args:   []string{"b"},
		},
		{
			input:  "a\nb\nc\n",
			output: "b\n",
			err:    nil,
			p:      params{fixed: true, expr: "b"},
		},
	}

	for idx, te := range tests {
		test := te
		t.Run(fmt.Sprintf("case_%d", idx), func(t *testing.T) {
			var stdout bytes.Buffer
			rc := io.NopCloser(strings.NewReader(test.input))
			cmd := cmd{
				stdin:  rc,
				stdout: bufio.NewWriter(&stdout),
				stderr: nil,
				params: test.p,
				args:   test.args,
			}
			err := cmd.run()
			if err != test.err {
				t.Errorf("got err %v, want %v", err, test.err)
			}

			res := stdout.String()
			if res != test.output {
				t.Errorf("got out %q, want %q", res, test.output)
			}
		})
	}
}

func TestFilesGrep(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	tmpDir := t.TempDir()
	f1, err := os.CreateTemp(tmpDir, "f1")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := os.CreateTemp(tmpDir, "f2")
	if err != nil {
		t.Fatal(err)
	}
	f3, err := os.CreateTemp(tmpDir, "f3")
	if err != nil {
		t.Fatal(err)
	}

	_, err = f1.WriteString("hix\nnix\n")
	_ = f1.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = f2.WriteString("hix\nhello\n")
	_ = f2.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = f3.WriteString("hix\ngoodbye\n")
	_ = f3.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		output  string
		err     error
		partial bool
		p       params
		args    []string
	}{
		{
			output: fmt.Sprintf("%s:hix\n%s:hix\n%s:hix\n", f1.Name(), f2.Name(), f3.Name()),
			err:    nil,
			p:      params{recursive: true},
			args:   []string{"hix", tmpDir},
		},
		{
			output: fmt.Sprintf("grep: %v: Is a directory\n", tmpDir),
			err:    filepath.SkipDir,
			p:      params{recursive: false},
			args:   []string{"hix", tmpDir},
		},
		{
			output: fmt.Sprintf("%s:hello\n", f2.Name()),
			err:    nil,
			args:   []string{"hello", f1.Name(), f2.Name()},
		},
		{
			output: fmt.Sprintf("%s\n", f1.Name()),
			err:    nil,
			p:      params{noShowMatch: true},
			args:   []string{"nix", f1.Name()},
		},
		{
			output: "",
			err:    nil,
			p:      params{quiet: true},
			args:   []string{"nix", f1.Name()},
		},
		{
			output:  fmt.Sprintf("grep: %s", tmpDir+"1"),
			partial: true,
			err:     errQuiet,
			p:       params{quiet: true},
			args:    []string{"nix", tmpDir + "1"},
		},
	}

	for idx, te := range tests {
		test := te
		t.Run(fmt.Sprintf("case_%d", idx), func(t *testing.T) {
			var stdout bytes.Buffer
			cmd := cmd{
				stdin:  nil,
				stdout: bufio.NewWriter(&stdout),
				stderr: &stdout,
				params: test.p,
				args:   test.args,
			}
			err := cmd.run()
			if test.err == nil && err != nil {
				t.Errorf("got %v, want nil", err)
			} else if err != nil {
				if !strings.Contains(err.Error(), test.err.Error()) {
					t.Errorf("got %v, want %v", err, test.err)
				}
			}

			res := stdout.String()
			if (test.partial && !strings.Contains(res, test.output)) || (!test.partial && res != test.output) {
				t.Errorf("got %v, want %v", res, test.output)
			}
		})
	}
}

func TestDefaultParams(t *testing.T) {
	var stdout bytes.Buffer
	rc := io.NopCloser(strings.NewReader("hix\n"))

	err := run(rc, &stdout, &stdout, []string{"grep", "."})
	if err != nil {
		t.Errorf("got err %v, want %v", err, nil)
	}

	res := stdout.String()
	if res != "hix\n" {
		t.Errorf("got out %q, want %q", res, "hix\n")
	}
}

func TestGrepMain(t *testing.T) {
	var out, errOut bytes.Buffer
	ioEnv := core.IO{
		Stdin:  strings.NewReader("apple\nbanana\napricot\n"),
		Stdout: &out,
		Stderr: &errOut,
	}
	code := Main(t.Context(), ioEnv, "^a")
	if code != 0 {
		t.Fatalf("Main returned %d, err: %s", code, errOut.String())
	}
	want := "apple\napricot\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}

	// Test no match returns exit code 1
	out.Reset()
	errOut.Reset()
	ioEnv.Stdin = strings.NewReader("apple\nbanana\n")
	code = Main(t.Context(), ioEnv, "^z")
	if code != 1 {
		t.Fatalf("expected code 1 on no match, got %d", code)
	}
}
