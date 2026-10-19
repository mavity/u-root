// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build plan9 || windows || wasip1

package id

func getProcessIDs(real bool) (uid, gid int, gids []int) {
	return -1, -1, nil
}
