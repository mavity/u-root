// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !windows && !darwin && !freebsd && !netbsd && !openbsd

package sync

func sync(data, filesystem bool, args []string) error {
	return nil
}
