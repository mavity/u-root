// Copyright 2013-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package ps

func loadPlatformProcesses(x bool) ([]*Process, *Process, error) {
	g := getAllGlobNames()
	n, err := getAllStatNames(g)
	if err != nil {
		return nil, nil, err
	}
	return doTable(n, x)
}
