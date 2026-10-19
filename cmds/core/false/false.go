// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package falsecmd returns 1.
//
// Synopsis:
//
//	false
//
// Description:
//
//	Command line arguments are ignored.
package falsecmd

import (
	"context"

	"github.com/u-root/u-root/pkg/core"
)

// Main runs the false command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) int {
	return 1
}
