// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// stty prints or changes terminal characteristics.
//
// Synopsis:
//
//	stty [pretty | dump | size | load <file> | raw | <options>...]
package stty

import (
	"context"
	"fmt"

	"github.com/u-root/u-root/pkg/core"
)

// Main executes the stty command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "stty: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ioEnv.Stdout, ioEnv.Stderr, ioEnv, args); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "stty: %v\n", err)
		return 1
	}
	return 0
}
