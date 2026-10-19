// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows || plan9 || wasip1

package nohup

func ignoreSIGHUP() {
	// SIGHUP does not exist on this platform; no-op.
}
