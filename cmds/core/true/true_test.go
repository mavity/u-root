// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package truecmd

import (
	"context"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

// Ensure 0 is returned.
func TestTrue(t *testing.T) {
	if code := Main(context.Background(), core.IO{}); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}
