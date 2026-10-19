// Copyright 2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package cpiocmd

import (
	"os"
	"testing"
)

func checkFileInfo(t *testing.T, ent *dirEnt, newFileInfo os.FileInfo) {
	t.Helper()
	sizeMatches := ent.FileInfo.IsDir() || ent.FileInfo.Size() == newFileInfo.Size()
	if ent.FileInfo.Name() != newFileInfo.Name() || !sizeMatches || ent.FileInfo.IsDir() != newFileInfo.IsDir() {
		t.Errorf("File %s mismatched: %v vs %v", ent.FileInfo.Name(), ent.FileInfo, newFileInfo)
	}
}
