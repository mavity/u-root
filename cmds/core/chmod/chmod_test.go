// Copyright 2016-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chmod

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestApplyModeExpressions(t *testing.T) {
	for _, tt := range []struct {
		name     string
		mode     string
		current  uint16
		umask    uint16
		isDir    bool
		want     uint16
		conflict bool
	}{
		{name: "octal special bits", mode: "4755", current: 0o644, want: 0o4755},
		{name: "omitted who plus respects umask", mode: "+w", current: 0o444, umask: 0o022, want: 0o644},
		{name: "explicit all ignores umask", mode: "a+w", current: 0o444, umask: 0o022, want: 0o666},
		{name: "omitted who minus preserves umasked bits", mode: "-w", current: 0o666, umask: 0o022, want: 0o466, conflict: true},
		{name: "omitted who equals filters requested bits", mode: "=rw", current: 0o777, umask: 0o022, want: 0o644},
		{name: "empty equals clears mode", mode: "=", current: 0o777, umask: 0o022, want: 0},
		{name: "copy permissions", mode: "g=u", current: 0o600, want: 0o660},
		{name: "sequential copy clauses", mode: "u=rw,g=u,o=g", current: 0o700, want: 0o666},
		{name: "multiple groups per clause", mode: "og+rX-w", current: 0o600, want: 0o644},
		{name: "X uses prior execute bit", mode: "a+X", current: 0o100, want: 0o111},
		{name: "X for directory", mode: "a+X", current: 0, isDir: true, want: 0o111},
		{name: "X not for non-executable file", mode: "a+X", current: 0, want: 0},
		{name: "symbolic special bits", mode: "u+s,g+s,o+t", current: 0o755, want: 0o7755},
		{name: "inapplicable special permissions", mode: "o+s,u+t,g+t", current: 0o755, want: 0o755},
	} {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseModeExpression(tt.mode)
			if err != nil {
				t.Fatalf("parseModeExpression(%q): %v", tt.mode, err)
			}
			got, conflict := expr.apply(tt.current, tt.isDir, tt.umask)
			if got != tt.want || conflict != tt.conflict {
				t.Fatalf("apply(%04o, umask %04o) = %04o, conflict %v; want %04o, conflict %v",
					tt.current, tt.umask, got, conflict, tt.want, tt.conflict)
			}
		})
	}
}

func TestParseModeExpressionRejectsInvalidModes(t *testing.T) {
	for _, mode := range []string{
		"",
		"77778",
		"10000",
		"rwx",
		"u+",
		"u+r,",
		"u+q",
		"u+ru",
		"g=ux",
	} {
		t.Run(strconv.Quote(mode), func(t *testing.T) {
			if _, err := parseModeExpression(mode); err == nil {
				t.Fatalf("parseModeExpression(%q) succeeded", mode)
			}
		})
	}
}

func TestPOSIXGoFileModeConversion(t *testing.T) {
	for _, mode := range []uint16{0, 0o644, 0o4755, 0o2750, 0o1777, 0o7777} {
		if got := fileModeToPOSIX(posixModeToFileMode(mode)); got != mode {
			t.Errorf("round-trip %04o = %04o", mode, got)
		}
	}
}

func TestRunChmodModesAndReporting(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("POSIX permission semantics are not available on this platform")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	ioEnv := core.IO{Stdout: &out, Stderr: io.Discard, Dir: dir}
	if err := run(context.Background(), ioEnv, "04755", "file"); err != nil {
		t.Fatalf("run chmod 04755: %v", err)
	}
	assertMode(t, path, 0o4755)

	out.Reset()
	if err := run(context.Background(), ioEnv, "-c", "04755", "file"); err != nil {
		t.Fatalf("run unchanged chmod with -c: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("-c output for unchanged mode = %q, want empty", out.String())
	}

	out.Reset()
	if err := run(context.Background(), ioEnv, "-c", "-v", "0600", "file"); err != nil {
		t.Fatalf("run chmod with -c -v: %v", err)
	}
	if !strings.Contains(out.String(), "changed from 4755") {
		t.Fatalf("-c -v output = %q, want verbose change report", out.String())
	}
	assertMode(t, path, 0o600)

	out.Reset()
	if err := run(context.Background(), ioEnv, "-v", "-c", "0600", "file"); err != nil {
		t.Fatalf("run chmod with -v -c: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("-v -c output for unchanged mode = %q, want empty", out.String())
	}

	refPath := filepath.Join(dir, "reference")
	if err := os.WriteFile(refPath, []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(refPath, posixModeToFileMode(0o2750)); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), ioEnv, "--reference=reference", "file"); err != nil {
		t.Fatalf("run chmod --reference: %v", err)
	}
	assertMode(t, path, 0o2750)
}

func TestRunChmodMultipleOperandsAndQuietErrors(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	last := filepath.Join(dir, "last")
	for _, path := range []string{first, last} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ioEnv := core.IO{Stdout: io.Discard, Stderr: io.Discard}

	err := run(context.Background(), ioEnv, "0600", first, filepath.Join(dir, "missing"), last)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run with missing middle operand error = %v, want os.ErrNotExist", err)
	}
	assertMode(t, first, 0o600)
	assertMode(t, last, 0o600)

	var stderr strings.Builder
	ioEnv.Stderr = &stderr
	code := Main(context.Background(), ioEnv, "-f", "0600", filepath.Join(dir, "missing"))
	if code == 0 {
		t.Fatal("chmod -f with missing target returned success")
	}
	if stderr.Len() != 0 {
		t.Fatalf("chmod -f stderr = %q, want empty", stderr.String())
	}

	stderr.Reset()
	code = Main(context.Background(), ioEnv, "-f", "9999", first)
	if code == 0 {
		t.Fatal("chmod -f with invalid mode returned success")
	}
	if stderr.Len() != 0 {
		t.Fatalf("chmod -f invalid-mode stderr = %q, want empty", stderr.String())
	}
	assertMode(t, first, 0o600)
}

func TestMainRejectsUnsupportedOptionsAndPrintsHelp(t *testing.T) {
	for _, option := range []string{"-H", "-L", "-P", "-h", "--dereference", "--no-dereference", "--from=0644"} {
		t.Run(option, func(t *testing.T) {
			var stderr strings.Builder
			ioEnv := core.IO{Stdout: io.Discard, Stderr: &stderr}
			if code := Main(context.Background(), ioEnv, option, "0644", "file"); code == 0 {
				t.Fatalf("Main(%q) returned success", option)
			}
			if stderr.Len() == 0 {
				t.Fatalf("Main(%q) emitted no diagnostic", option)
			}
		})
	}

	var stdout strings.Builder
	ioEnv := core.IO{Stdout: &stdout, Stderr: io.Discard}
	if code := Main(context.Background(), ioEnv, "--help"); code != 0 {
		t.Fatalf("Main(--help) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "chmod") {
		t.Fatalf("--help output = %q", stdout.String())
	}

	stdout.Reset()
	if code := Main(context.Background(), ioEnv, "--version"); code != 0 {
		t.Fatalf("Main(--version) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "u-root") {
		t.Fatalf("--version output = %q", stdout.String())
	}
}

func TestRecursivePreserveRoot(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("root protection test uses the Linux guest root")
	}
	var stderr strings.Builder
	ioEnv := core.IO{Stdout: io.Discard, Stderr: &stderr}
	if code := Main(context.Background(), ioEnv, "-R", "0000", string(os.PathSeparator)); code == 0 {
		t.Fatal("recursive chmod on root unexpectedly succeeded")
	}
	if !strings.Contains(stderr.String(), "refusing to operate recursively on root") {
		t.Fatalf("root protection diagnostic = %q", stderr.String())
	}
}

func assertMode(t *testing.T, path string, want uint16) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if filepath.Separator == '/' {
		if got := fileModeToPOSIX(info.Mode()); got != want {
			t.Fatalf("mode of %q = %04o, want %04o", path, got, want)
		}
	}
}
