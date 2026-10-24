// Copyright 2016-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !unix && !wasip1

package chmod

func currentGuestUmask() (int, error) {
	return 0o022, nil
}
