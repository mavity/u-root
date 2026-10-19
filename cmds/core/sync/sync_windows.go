// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package sync

import (
	"fmt"
	"os"
	"syscall"
)

func sync(data, filesystem bool, args []string) error {
	switch {
	case data || filesystem:
		for _, fileName := range args {
			f, err := os.OpenFile(fileName, os.O_RDWR, 0o644)
			if err != nil {
				// try read-only if RDWR fails
				f, err = os.OpenFile(fileName, os.O_RDONLY, 0o644)
				if err != nil {
					return err
				}
			}
			err = syscall.FlushFileBuffers(syscall.Handle(f.Fd()))
			f.Close()
			if err != nil {
				return fmt.Errorf("FlushFileBuffers %s: %w", fileName, err)
			}
		}
		return nil
	default:
		// Standalone sync with no arguments flushes all file handles if any are passed.
		for _, fileName := range args {
			f, err := os.OpenFile(fileName, os.O_RDONLY, 0o644)
			if err != nil {
				return err
			}
			_ = syscall.FlushFileBuffers(syscall.Handle(f.Fd()))
			f.Close()
		}
		return nil
	}
}
