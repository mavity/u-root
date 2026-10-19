// Copyright 2012-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !plan9 && !windows && !wasip1

package hostname

import "errors"

func Sethostname(n string) error {
	return errors.New("setting hostname not supported on this platform")
}
