// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rg

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func run(t *testing.T, dir, input string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Main(context.Background(), core.IO{
		Dir: dir, Stdin: strings.NewReader(input), StdinIsTerminal: input == "",
		StdoutIsTerminal: true,
		Stdout:           &stdout, Stderr: &stderr,
		LookupEnv: func(key string) (string, bool) {
			if key == "NO_COLOR" {
				return "1", true
			}
			return "", false
		},
	}, args...)
	return code, stdout.String(), stderr.String()
}

func TestMainFileSearchOutputAndStatus(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("needle\nother\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, dir, "", "-F", "needle", "sample.txt")
	if code != 0 || stderr != "" || stdout != "sample.txt:1:needle\n" {
		t.Fatalf("got code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, dir, "", "-n", "-F", "needle", "sample.txt")
	if code != 0 || stderr != "" || stdout != "sample.txt:1:needle\n" {
		t.Fatalf("explicit line number: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, dir, "", "absent", "sample.txt")
	if code != 1 || stdout != "" || stderr != "" {
		t.Fatalf("no-match: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestRecursiveSearchHonorsHiddenIgnoreAndGlob(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		".gitignore":      "ignored.go\n",
		".rgignore":       "!ignored.go\n",
		"visible.go":      "needle\n",
		"ignored.go":      "needle\n",
		"excluded.txt":    "needle\n",
		".hidden.go":      "needle\n",
		"nested/other.go": "needle\n",
	}
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := run(t, dir, "", "-g", "*.go", "needle")
	if code != 0 || stderr != "" {
		t.Fatalf("search: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "ignored.go:1:needle") || strings.Contains(stdout, ".hidden.go") || !strings.Contains(stdout, "visible.go:1:needle") || !strings.Contains(stdout, "nested/other.go:1:needle") {
		t.Fatalf("unexpected recursive output: %q", stdout)
	}

	code, stdout, stderr = run(t, dir, "", "--hidden", "--no-ignore", "-g", "**/*.go", "needle")
	if code != 0 || stderr != "" || !strings.Contains(stdout, ".hidden.go:1:needle") {
		t.Fatalf("hidden/no-ignore search: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, dir, "", "-g", "!*.txt", "needle")
	if code != 0 || stderr != "" || strings.Contains(stdout, "excluded.txt") || !strings.Contains(stdout, "ignored.go") {
		t.Fatalf("negated glob: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestCompressedMultilineStatsAndCaseInsensitiveGlobs(t *testing.T) {
	dir := t.TempDir()
	compressed, err := os.Create(filepath.Join(dir, "packed.txt.gz"))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(compressed)
	if _, err := gz.Write([]byte("needle in gzip\n")); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "multiline.txt"), []byte("first\nsecond\nlast\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible.go"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run(t, dir, "", "-z", "needle", "packed.txt.gz")
	if code != 0 || stderr != "" || stdout != "packed.txt.gz:1:needle in gzip\n" {
		t.Fatalf("compressed search: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	code, stdout, stderr = run(t, dir, "", "-U", "first\nsecond", "multiline.txt")
	if code != 0 || stderr != "" || stdout != "multiline.txt:1:first\nmultiline.txt:2:second\n" {
		t.Fatalf("multiline search: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, dir, "first\nsecond\n", "-U", "first\nsecond")
	if code != 0 || stderr != "" || stdout != "1:first\n2:second\n" {
		t.Fatalf("multiline stdin search: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, dir, "first\nsecond\n", "-U", "-n", "first\nsecond")
	if code != 0 || stderr != "" || stdout != "1:first\n2:second\n" {
		t.Fatalf("numbered multiline stdin search: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	code, stdout, stderr = run(t, dir, "", "--glob-case-insensitive", "-g", "*.GO", "needle")
	if code != 0 || stderr != "" || stdout != "visible.go:1:needle\n" {
		t.Fatalf("case-insensitive glob: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	code, stdout, stderr = run(t, dir, "", "--stats", "needle", "visible.go")
	if code != 0 || stderr != "" ||
		!strings.Contains(stdout, "visible.go:1:needle\n\n1 matches\n") ||
		!strings.Contains(stdout, "1 files searched\n") ||
		!strings.Contains(stdout, "bytes printed\n") ||
		!strings.Contains(stdout, "bytes searched\n") {
		t.Fatalf("stats: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestGlobalAndGitInfoExcludeRules(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	gitInfo := filepath.Join(dir, ".git", "info")
	if err := os.MkdirAll(gitInfo, 0o700); err != nil {
		t.Fatal(err)
	}
	globalExclude := filepath.Join(home, "global.ignore")
	if err := os.WriteFile(globalExclude, []byte("global.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[core]\n\texcludesfile = "+globalExclude+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitInfo, "exclude"), []byte("info.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"global.txt": "needle\n",
		"info.txt":   "needle\n",
		"kept.txt":   "needle\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	code := Main(context.Background(), core.IO{
		Dir: dir, Stdout: &stdout, Stderr: &stderr,
		LookupEnv: func(key string) (string, bool) {
			if key == "HOME" {
				return home, true
			}
			return "", false
		},
	}, "needle", ".")
	if code != 0 || stderr.Len() != 0 || stdout.String() != "kept.txt:needle\n" {
		t.Fatalf("global excludes: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestJSONOutputIsStructured(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run(t, dir, "", "--json", "needle", "a.txt")
	if code != 0 || stderr != "" {
		t.Fatalf("json search: code=%d stderr=%q", code, stderr)
	}
	var types []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("invalid JSON event %q: %v", line, err)
		}
		types = append(types, event.Type)
	}
	if strings.Join(types, ",") != "begin,match,end,summary" {
		t.Fatalf("event types = %v, want begin,match,end,summary", types)
	}
}

func TestPerlLookaroundSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle badneedle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, dir, "", "-P", `(?<!bad)needle`, "a.txt")
	if code != 0 || stderr != "" || stdout != "a.txt:1:needle badneedle\n" {
		t.Fatalf("lookbehind search: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestOutputModesAndLongRecords(t *testing.T) {
	long := strings.Repeat("x", 128*1024) + "needle\nlast\n"
	code, stdout, stderr := run(t, "", long, "-n", "--column", "needle", "-")
	if code != 0 || stderr != "" || stdout != "1:131073:"+strings.Repeat("x", 128*1024)+"needle\n" {
		t.Fatalf("long record: code=%d output length=%d output prefix=%q stderr=%q", code, len(stdout), stdout[:min(len(stdout), 80)], stderr)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("one needle needle\nnone\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = run(t, dir, "", "-o", "-c", "needle", "data.txt")
	if code != 0 || stderr != "" || stdout != "data.txt:1\n" {
		t.Fatalf("count mode: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, dir, "", "--count-matches", "needle", "data.txt")
	if code != 0 || stderr != "" || stdout != "data.txt:2\n" {
		t.Fatalf("count-matches: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestFilesListingAndPatternFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "patterns"), []byte("needle\nother\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, dir, "", "--files", ".")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "a.txt") {
		t.Fatalf("--files: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, dir, "", "-f", "patterns", "a.txt")
	if code != 0 || stderr != "" || stdout != "a.txt:1:needle\n" {
		t.Fatalf("-f: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestMaxCountZeroAndStdinPatternFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle\nagain needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, dir, "", "-c", "-m", "0", "needle", "a.txt")
	if code != 1 || stderr != "" || stdout != "a.txt:0\n" {
		t.Fatalf("zero max-count: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, dir, "needle\nother\n", "-f", "-", "a.txt")
	if code != 0 || stderr != "" || stdout != "a.txt:1:needle\na.txt:2:again needle\n" {
		t.Fatalf("stdin pattern file: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestNullFilenameSeparator(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, dir, "", "--null", "needle", "a.txt")
	if code != 0 || stderr != "" || stdout != "a.txt\x001:needle\n" {
		t.Fatalf("null filename: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
