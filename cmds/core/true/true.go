// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package truecmd returns 0.
//
// Synopsis:
//
//	true
//
// Description:
//
//	Command line arguments are ignored.
package truecmd

import (
	"context"

	"github.com/u-root/u-root/pkg/core"
)

// Main runs the true command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) int {
	return 0
}
