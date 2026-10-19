// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chmod

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestChmod(t *testing.T) {
	tmpdir := t.TempDir()
	filePath := filepath.Join(tmpdir, "tmpfile")
	if err := os.WriteFile(filePath, []byte("data"), 0o666); err != nil {
		t.Fatalf("Failed to create tmp file: %v", err)
	}

	for _, tt := range []struct {
		name       string
		args       []string
		recursive  bool
		reference  string
		modeBefore os.FileMode
		modeAfter  os.FileMode
		err        error
	}{
		{
			name: "len(args) < 1",
			err:  errBadUsage,
		},
		{
			name: "len(args) < 2 && *reference",
			args: []string{"arg"},
			err:  errBadUsage,
		},
		{
			name: "file does not exist",
			args: []string{"g-rx", "filedoesnotexist"},
			err:  os.ErrNotExist,
		},
		{
			name: "Value should be less than or equal to 0777",
			args: []string{"7777", filePath},
			err:  strconv.ErrRange,
		},
		{
			name:       "mode 0777 correct",
			args:       []string{"0777", filePath},
			modeBefore: 0o000,
			modeAfter:  0o777,
		},
		{
			name:       "mode 0644 correct",
			args:       []string{"0644", filePath},
			modeBefore: 0o777,
			modeAfter:  0o644,
		},
		{
			name: "unable to decode mode",
			args: []string{"a=9rwx", filePath},
			err:  strconv.ErrSyntax,
		},
		{
			name:       "mode u-rwx correct",
			args:       []string{"u-rwx", filePath},
			modeBefore: 0o777,
			modeAfter:  0o077,
		},
		{
			name:       "mode g-rx correct",
			args:       []string{"g-rx", filePath},
			modeBefore: 0o777,
			modeAfter:  0o727,
		},
		{
			name:       "mode a-xr correct",
			args:       []string{"a-xr", filePath},
			modeBefore: 0o222,
			modeAfter:  0o222,
		},
		{
			name:       "mode a-xw correct",
			args:       []string{"a-xw", filePath},
			modeBefore: 0o666,
			modeAfter:  0o444,
		},
		{
			name:       "mode u-xw correct",
			args:       []string{"u-xw", filePath},
			modeBefore: 0o666,
			modeAfter:  0o466,
		},
		{
			name:       "mode a= correct",
			args:       []string{"a=", filePath},
			modeBefore: 0o777,
			modeAfter:  0o000,
		},
		{
			name:       "mode u= correct",
			args:       []string{"u=", filePath},
			modeBefore: 0o777,
			modeAfter:  0o077,
		},
		{
			name:       "mode u- correct",
			args:       []string{"u-", filePath},
			modeBefore: 0o777,
			modeAfter:  0o777,
		},
		{
			name:       "mode o+ correct",
			args:       []string{"o+", filePath},
			modeBefore: 0o700,
			modeAfter:  0o700,
		},
		{
			name:       "mode g=rx correct",
			args:       []string{"g=rx", filePath},
			modeBefore: 0o777,
			modeAfter:  0o757,
		},
		{
			name:       "mode u=rx correct",
			args:       []string{"u=rx", filePath},
			modeBefore: 0o077,
			modeAfter:  0o577,
		},
		{
			name:       "mode o=rx correct",
			args:       []string{"o=rx", filePath},
			modeBefore: 0o077,
			modeAfter:  0o075,
		},
		{
			name:       "mode u=xw correct",
			args:       []string{"u=xw", filePath},
			modeBefore: 0o742,
			modeAfter:  0o342,
		},
		{
			name:       "mode a-rwx correct",
			args:       []string{"a-rwx", filePath},
			modeBefore: 0o777,
			modeAfter:  0o000,
		},
		{
			name:       "mode a-rx correct",
			args:       []string{"a-rx", filePath},
			modeBefore: 0o777,
			modeAfter:  0o222,
		},
		{
			name:       "mode a-x correct",
			args:       []string{"a-x", filePath},
			modeBefore: 0o777,
			modeAfter:  0o666,
		},
		{
			name:       "mode o+rwx correct",
			args:       []string{"o+rwx", filePath},
			modeBefore: 0o000,
			modeAfter:  0o007,
		},
		{
			name:       "mode a+rwx correct",
			args:       []string{"a+rwx", filePath},
			modeBefore: 0o000,
			modeAfter:  0o777,
		},
		{
			name:       "mode a+xrw correct",
			args:       []string{"a+xrw", filePath},
			modeBefore: 0o000,
			modeAfter:  0o777,
		},
		{
			name:       "mode a+xxxxxxxx correct",
			args:       []string{"a+xxxxxxxx", filePath},
			modeBefore: 0o000,
			modeAfter:  0o111,
		},
		{
			name:       "mode o+xxxxx correct",
			args:       []string{"o+xxxxx", filePath},
			modeBefore: 0o000,
			modeAfter:  0o001,
		},
		{
			name:       "mode a+rx correct",
			args:       []string{"a+rx", filePath},
			modeBefore: 0o000,
			modeAfter:  0o555,
		},
		{
			name:       "mode a+r correct",
			args:       []string{"a+r", filePath},
			modeBefore: 0o111,
			modeAfter:  0o555,
		},
		{
			name:       "mode a=rwx correct",
			args:       []string{"a=rwx", filePath},
			modeBefore: 0o000,
			modeAfter:  0o777,
		},
		{
			name:       "mode a=rx correct",
			args:       []string{"a=rx", filePath},
			modeBefore: 0o000,
			modeAfter:  0o555,
		},
		{
			name:      "bad reference file",
			args:      []string{"a=rx", filePath},
			reference: "filedoesnotexist",
			err:       os.ErrNotExist,
		},
		{
			name:       "correct reference file",
			args:       []string{filePath},
			modeBefore: 0o222,
			modeAfter:  0o222,
			reference:  filePath,
		},
		{
			name:      "bad filepath",
			args:      []string{"a=rx", "pathdoes not exist"},
			recursive: true,
			err:       os.ErrNotExist,
		},
		{
			name:       "correct path filepath",
			args:       []string{"0777", filePath},
			recursive:  true,
			modeBefore: 0o777,
			modeAfter:  0o777,
			err:        nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Chmod(filePath, tt.modeBefore)

			var cmdArgs []string
			if tt.recursive {
				cmdArgs = append(cmdArgs, "-recursive")
			}
			if tt.reference != "" {
				cmdArgs = append(cmdArgs, "-reference", tt.reference)
			}
			cmdArgs = append(cmdArgs, tt.args...)

			ioEnv := core.IO{Stdout: io.Discard, Stderr: io.Discard}
			err := run(context.Background(), ioEnv, cmdArgs...)
			if !errors.Is(err, tt.err) {
				t.Errorf("chmod(%v, %q, %q) = %v, want %v", tt.recursive, tt.reference, tt.args, err, tt.err)
				return
			}

			// On POSIX systems, verify permission bits match exactly
			if filepath.Separator == '/' && tt.err == nil {
				fi, err := os.Stat(filePath)
				if err != nil {
					t.Fatalf("failed to stat file: %v", err)
				}
				if fi.Mode() != tt.modeAfter {
					t.Errorf("chmod(%v, %q, %q) = mode = %o, want %o", tt.recursive, tt.reference, tt.args, fi.Mode(), tt.modeAfter)
				}
			}
		})
	}
}

func TestMultipleFiles(t *testing.T) {
	d1 := t.TempDir()
	f1 := filepath.Join(d1, "tmpfile1")
	if err := os.WriteFile(f1, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	d2 := t.TempDir()
	f2 := filepath.Join(d2, "tmpfile2")
	if err := os.WriteFile(f2, []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}

	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdout: io.Discard, Stderr: stderr}

	err := run(context.Background(), ioEnv, "0777", f1, "filenotexists", f2)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}

	if filepath.Separator == '/' {
		fi, err := os.Stat(f1)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if fi.Mode() != 0o777 {
			t.Errorf("chmod(%q) = %o, want %o", f1, fi.Mode(), 0o777)
		}

		fi, err = os.Stat(f2)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if fi.Mode() != 0o777 {
			t.Errorf("chmod(%q) = %o, want %o", f2, fi.Mode(), 0o777)
		}
	}
}
