// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package tty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func findTTY(stdin io.Reader) (string, error) {
	f, ok := stdin.(*os.File)
	if !ok {
		f = os.Stdin
	}
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}

	s, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("stat stdin:Sys() is empty: %w", os.ErrNotExist)
	}

	var found string
	err = filepath.WalkDir("/dev", func(path string, dir os.DirEntry, _ error) error {
		switch path {
		case "/dev/fd":
			if dir.IsDir() {
				return filepath.SkipDir
			}
		case "/dev/stdin", "/dev/stdout", "/dev/stderr":
			return nil
		}
		if fi, err := os.Stat(path); err == nil {
			stat, ok := fi.Sys().(*syscall.Stat_t)
			if ok {
				if stat.Ino == s.Ino && stat.Dev == s.Dev {
					found = path
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", errors.New("not a tty")
	}
	return found, nil
}
