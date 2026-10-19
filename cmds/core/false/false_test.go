// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package falsecmd

import (
	"context"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestFalse(t *testing.T) {
	if code := Main(context.Background(), core.IO{}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}
