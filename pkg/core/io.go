// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package core defines the in-process execution environment for u-root commands.
package core

import (
	"context"
	"io"
	"path/filepath"
)

// LookupEnvFunc looks up an environment variable in the execution context.
type LookupEnvFunc func(key string) (string, bool)

// IO defines the execution environment injected into in-process commands.
type IO struct {
	Stdin            io.Reader
	Stdout           io.Writer
	Stderr           io.Writer
	Dir              string
	StdoutIsTerminal bool
	TerminalWidth    int
	LookupEnv        LookupEnvFunc
	Setenv           func(key, value string) error
	Environ          func() []string
	Runner           func(ctx context.Context, ioEnv IO, args ...string) int
	IsBuiltin        func(name string) bool
}

// ResolvePath resolves relative paths against ioEnv.Dir without mutating process cwd.
func (ioEnv IO) ResolvePath(p string) string {
	if p == "" || filepath.IsAbs(p) || ioEnv.Dir == "" {
		return p
	}
	return filepath.Join(ioEnv.Dir, p)
}

// Getenv retrieves an environment variable via LookupEnv or returns an empty string.
func (ioEnv IO) Getenv(key string) string {
	if ioEnv.LookupEnv != nil {
		if v, ok := ioEnv.LookupEnv(key); ok {
			return v
		}
	}
	return ""
}

// SetenvVal sets an environment variable via Setenv if defined.
func (ioEnv IO) SetenvVal(key, value string) error {
	if ioEnv.Setenv != nil {
		return ioEnv.Setenv(key, value)
	}
	return nil
}

// MainFunc is the signature of reentrant command entrypoints in cmds/core/<cmd>.
type MainFunc func(ctx context.Context, ioEnv IO, args ...string) int
