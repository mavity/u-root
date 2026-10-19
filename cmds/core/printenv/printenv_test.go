// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package printenv

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestPrintenv(t *testing.T) {
	fakeEnv := map[string]string{
		"GIRAFFE": "akaros",
		"GOPHER":  "go",
		"PENGUIN": "linux",
	}

	ioEnv := core.IO{
		Environ: func() []string {
			return []string{"GIRAFFE=akaros", "GOPHER=go", "PENGUIN=linux"}
		},
		LookupEnv: func(k string) (string, bool) {
			v, ok := fakeEnv[k]
			return v, ok
		},
	}

	t.Run("all", func(t *testing.T) {
		var buf bytes.Buffer
		env := ioEnv
		env.Stdout = &buf

		if code := Main(context.Background(), env); code != 0 {
			t.Fatalf("expected 0, got %d", code)
		}

		out := buf.String()
		for _, want := range []string{"GIRAFFE=akaros", "GOPHER=go", "PENGUIN=linux"} {
			if !strings.Contains(out, want) {
				t.Errorf("output does not contain %s; got %s", want, out)
			}
		}
	})

	t.Run("arg", func(t *testing.T) {
		var buf bytes.Buffer
		env := ioEnv
		env.Stdout = &buf

		if code := Main(context.Background(), env, "GOPHER"); code != 0 {
			t.Fatalf("expected 0, got %d", code)
		}
		if r := buf.String(); r != "go\n" {
			t.Errorf("want `go\n`, got %s", r)
		}
	})

	t.Run("missing", func(t *testing.T) {
		var buf bytes.Buffer
		env := ioEnv
		env.Stdout = &buf

		if code := Main(context.Background(), env, "NONEXISTENT_VAR"); code != 1 {
			t.Fatalf("expected 1, got %d", code)
		}
	})
}
