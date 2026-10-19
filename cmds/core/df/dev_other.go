// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build plan9 || wasip1

package df

func deviceNumber(path string) (uint64, error) {
	return 1, nil
}
