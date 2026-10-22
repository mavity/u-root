// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package touch

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/u-root/u-root/pkg/core"
)

func TestParseParamsDate(t *testing.T) {
	date := "2021-01-01T00:00:00Z"
	expected, err := time.Parse(time.RFC3339, date)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	p, err := parseParams(date, "", "", false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !expected.Equal(p.time) {
		t.Errorf("expected %v, got %v", expected, p.time)
	}

	date = "invalid"
	_, err = parseParams(date, "", "", false, false, false)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestParseParams(t *testing.T) {
	var tests = []struct {
		expected     params
		access       bool
		modification bool
		create       bool
	}{
		{
			access:       false,
			modification: false,
			create:       false,
			expected: params{
				access:       true,
				modification: true,
				create:       false,
			},
		},
		{
			access:       true,
			modification: false,
			create:       false,
			expected: params{
				access:       true,
				modification: false,
				create:       false,
			},
		},
		{
			access:       false,
			modification: true,
			create:       true,
			expected: params{
				access:       false,
				modification: true,
				create:       true,
			},
		},
	}

	for _, test := range tests {
		p, err := parseParams("", "", "", test.access, test.modification, test.create)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if p.access != test.expected.access {
			t.Errorf("expected %v, got %v", test.expected.access, p.access)
		}
		if p.modification != test.expected.modification {
			t.Errorf("expected %v, got %v", test.expected.modification, p.modification)
		}
		if p.create != test.expected.create {
			t.Errorf("expected %v, got %v", test.expected.create, p.create)
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	now := time.Date(2025, time.June, 15, 12, 0, 0, 0, time.Local)
	for _, tt := range []struct {
		stamp string
		want  time.Time
	}{
		{"01011234", time.Date(2025, time.January, 1, 12, 34, 0, 0, time.Local)},
		{"6801010000", time.Date(2068, time.January, 1, 0, 0, 0, 0, time.Local)},
		{"6901010000", time.Date(1969, time.January, 1, 0, 0, 0, 0, time.Local)},
		{"202001020304.05", time.Date(2020, time.January, 2, 3, 4, 5, 0, time.Local)},
	} {
		got, err := parseTimestamp(tt.stamp, now)
		if err != nil {
			t.Errorf("parseTimestamp(%q) error = %v", tt.stamp, err)
			continue
		}
		if !got.Equal(tt.want) {
			t.Errorf("parseTimestamp(%q) = %v, want %v", tt.stamp, got, tt.want)
		}
	}

	for _, stamp := range []string{"", "202013010000", "202002300000", "202001010000.60", "202001010000.0"} {
		if _, err := parseTimestamp(stamp, now); err == nil {
			t.Errorf("parseTimestamp(%q) succeeded, want error", stamp)
		}
	}
}

func TestTouchTimestampAndReference(t *testing.T) {
	dir := t.TempDir()
	ioEnv := core.IO{Dir: dir, Stdout: io.Discard, Stderr: io.Discard}

	if code := Main(context.Background(), ioEnv, "-t", "200101020304.05", "timestamp"); code != 0 {
		t.Fatalf("touch -t exit code = %d", code)
	}
	timestamp, err := parseTimestamp("200101020304.05", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "timestamp"))
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().Unix() != timestamp.Unix() {
		t.Errorf("touch -t mtime = %v, want %v", info.ModTime(), timestamp)
	}

	reference := filepath.Join(dir, "reference")
	if err := os.WriteFile(reference, []byte("reference"), 0o600); err != nil {
		t.Fatal(err)
	}
	referenceAccess := time.Date(2001, time.February, 3, 4, 5, 6, 0, time.Local)
	referenceModify := time.Date(2002, time.March, 4, 5, 6, 7, 0, time.Local)
	if err := os.Chtimes(reference, referenceAccess, referenceModify); err != nil {
		t.Fatal(err)
	}
	if code := Main(context.Background(), ioEnv, "-r", "reference", "copied-times"); code != 0 {
		t.Fatalf("touch -r exit code = %d", code)
	}
	copied, err := os.Stat(filepath.Join(dir, "copied-times"))
	if err != nil {
		t.Fatal(err)
	}
	copiedAccess, err := referenceAccessTime(copied)
	if err != nil {
		t.Fatal(err)
	}
	if copiedAccess.Unix() != referenceAccess.Unix() {
		t.Errorf("touch -r atime = %v, want %v", copiedAccess, referenceAccess)
	}
	if copied.ModTime().Unix() != referenceModify.Unix() {
		t.Errorf("touch -r mtime = %v, want %v", copied.ModTime(), referenceModify)
	}

	partial := filepath.Join(dir, "partial")
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	initialAccess := time.Date(2003, time.April, 5, 6, 7, 8, 0, time.Local)
	initialModify := time.Date(2004, time.May, 6, 7, 8, 9, 0, time.Local)
	if err := os.Chtimes(partial, initialAccess, initialModify); err != nil {
		t.Fatal(err)
	}
	if code := Main(context.Background(), ioEnv, "-a", "-r", "reference", "partial"); code != 0 {
		t.Fatalf("touch -a -r exit code = %d", code)
	}
	partialInfo, err := os.Stat(partial)
	if err != nil {
		t.Fatal(err)
	}
	partialAccess, err := referenceAccessTime(partialInfo)
	if err != nil {
		t.Fatal(err)
	}
	if partialAccess.Unix() != referenceAccess.Unix() {
		t.Errorf("touch -a -r atime = %v, want %v", partialAccess, referenceAccess)
	}
	if partialInfo.ModTime().Unix() != initialModify.Unix() {
		t.Errorf("touch -a -r changed mtime to %v, want %v", partialInfo.ModTime(), initialModify)
	}

	if err := os.Chtimes(partial, initialAccess, initialModify); err != nil {
		t.Fatal(err)
	}
	if code := Main(context.Background(), ioEnv, "-m", "-r", "reference", "partial"); code != 0 {
		t.Fatalf("touch -m -r exit code = %d", code)
	}
	partialInfo, err = os.Stat(partial)
	if err != nil {
		t.Fatal(err)
	}
	partialAccess, err = referenceAccessTime(partialInfo)
	if err != nil {
		t.Fatal(err)
	}
	if partialAccess.Unix() != initialAccess.Unix() {
		t.Errorf("touch -m -r changed atime to %v, want %v", partialAccess, initialAccess)
	}
	if partialInfo.ModTime().Unix() != referenceModify.Unix() {
		t.Errorf("touch -m -r mtime = %v, want %v", partialInfo.ModTime(), referenceModify)
	}
}

func TestTouchEmptyDir(t *testing.T) {
	ts := "2021-01-01T00:00:00Z"
	datetimeArg, _ := time.Parse(time.RFC3339, ts)

	var tests = []struct {
		runErr     error
		commandErr error
		name       string
		p          params
		args       []string
		fileArgs   []string
	}{
		{
			name:     "create is true, no new files created",
			args:     []string{"-amc", "-d", ts},
			fileArgs: []string{"a1", "a2"},
			p: params{
				access:       true,
				modification: true,
				create:       true,
				time:         datetimeArg,
			},
		},
		{
			name:     "create is false, files should be created",
			args:     []string{"-a", "-m", "-d", ts},
			fileArgs: []string{"a1", "a2"},
			p: params{
				access:       true,
				modification: true,
				create:       false,
				time:         datetimeArg,
			},
		},
		{
			name:     "no such file or directory",
			args:     []string{},
			fileArgs: []string{"no/such/file/or/directory"},
			p: params{
				create: false,
			},
			runErr: os.ErrNotExist,
		},
		{
			name: "no files specified",
			args: []string{},
			p: params{
				create: false,
			},
			commandErr: errNoFiles,
		},
	}

	for _, test := range tests {
		temp := t.TempDir()
		var fileArgs []string
		for _, arg := range test.fileArgs {
			fileArgs = append(fileArgs, filepath.Join(temp, arg))
		}

		ioEnv := core.IO{Stdout: io.Discard, Stderr: io.Discard}
		c, err := parseCommand(ioEnv, append(test.args, fileArgs...)...)
		if !errors.Is(err, test.commandErr) {
			t.Fatalf("command() expected %v, got %v", test.commandErr, err)
		}
		if test.commandErr != nil {
			continue
		}

		err = c.run(context.Background(), ioEnv)
		if !errors.Is(err, test.runErr) {
			t.Fatalf("command() expected %v, got %v", test.runErr, err)
		}
		if test.runErr != nil {
			continue
		}

		for _, arg := range fileArgs {
			_, err := os.Stat(arg)
			if test.p.create {
				if !os.IsNotExist(err) {
					t.Errorf("expected %s to not exist", arg)
				}
			} else {
				if err != nil {
					t.Errorf("expected %s to exist, got %v", arg, err)
				}

				stat, err := os.Stat(arg)
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}

				if test.p.modification {
					if stat.ModTime().Unix() != test.p.time.Unix() {
						t.Errorf("expected %s to have mod time %v, got %v", arg, test.p.time, stat.ModTime())
					}
				}
			}
		}
	}
}
