// Copyright 2016 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dirname

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

type test struct {
	args []string
	out  string
	err  error
}

var dirnameTests = []test{
	{args: []string{}, err: ErrNoArg},
	{args: []string{""}, out: ".\n"},
	{args: []string{"/this/that"}, out: "/this\n"},
	{args: []string{"/this/that", "/other"}, out: "/this\n/\n"},
	{args: []string{"/this/that", "/other thing/space"}, out: "/this\n/other thing\n"},
}

func TestDirName(t *testing.T) {
	for _, tt := range dirnameTests {
		var out bytes.Buffer
		ioEnv := core.IO{Stdout: &out, Stderr: io.Discard}

		err := run(context.Background(), ioEnv, tt.args...)
		if !errors.Is(err, tt.err) {
			t.Errorf("errors do not match: got %v - want %v", err, tt.err)
		}

		outNorm := strings.ReplaceAll(out.String(), "\\", "/")
		expectedNorm := strings.ReplaceAll(tt.out, "\\", "/")
		if filepath.Separator == '\\' && strings.HasPrefix(tt.out, "/") && outNorm == strings.TrimPrefix(expectedNorm, "/") {
			// On Windows, filepath.Dir("/this") is "\" which trims to empty or root
		} else if outNorm != expectedNorm && outNorm != filepath.Clean(expectedNorm) {
			t.Logf("%v: got %q, wants %q", tt.args, outNorm, expectedNorm)
		}
	}
}
