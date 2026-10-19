// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip1

package sshd

import (
	"context"
	"fmt"

	"github.com/u-root/u-root/pkg/core"
)

// Main runs sshd on wasip1 (unsupported).
func Main(ctx context.Context, ioEnv core.IO, args ...string) int {
	fmt.Fprintln(ioEnv.Stderr, "sshd: SSH daemon is not supported on wasip1")
	return 1
}
