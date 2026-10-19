// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !freebsd && !linux && !darwin

package ldd

import (
	"debug/elf"
	"os"
)

// List returns library dependencies of ELF files using debug/elf.
func List(names ...string) ([]string, error) {
	var allLibs []string
	seen := make(map[string]bool)
	for _, n := range names {
		if _, err := os.Stat(n); err != nil {
			return nil, err
		}
		f, err := elf.Open(n)
		if err != nil {
			continue
		}
		libs, err := f.ImportedLibraries()
		f.Close()
		if err != nil {
			continue
		}
		for _, lib := range libs {
			if !seen[lib] {
				seen[lib] = true
				allLibs = append(allLibs, lib)
			}
		}
	}
	return allLibs, nil
}

// FList returns library dependencies for the given files.
func FList(names ...string) ([]string, error) {
	return List(names...)
}
