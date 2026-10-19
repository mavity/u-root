// Copyright 2017-2019 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package yes

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/u-root/u-root/pkg/core"
)

func TestYes(t *testing.T) {
	count := uint64(5)
	for _, tt := range []struct {
		name     string
		in       []string
		expected string
	}{
		{
			name:     "noParameterCloseTest",
			in:       []string{},
			expected: "y\ny\ny\ny\ny\n",
		},
		{
			name:     "oneParameterCloseTest",
			in:       []string{"hi"},
			expected: "hi\nhi\nhi\nhi\nhi\n",
		},
		{
			name:     "fourParameterCloseTest",
			in:       []string{"hi", "how", "are", "you"},
			expected: "hi how are you\nhi how are you\nhi how are you\nhi how are you\nhi how are you\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := runYes(&buf, count, tt.in...); err != nil {
				t.Error(err)
			}
			if buf.String() != tt.expected {
				t.Errorf("%s does not match expected %s", buf.String(), tt.expected)
			}
		})
	}
}

func TestMainInvocation(t *testing.T) {
	t.Run("pipe close terminates cleanly", func(t *testing.T) {
		r, w := io.Pipe()
		var stderr bytes.Buffer
		ioEnv := core.IO{Stdout: w, Stderr: &stderr}

		done := make(chan int, 1)
		go func() {
			done <- Main(context.Background(), ioEnv, "test")
		}()

		buf := make([]byte, 10)
		_, _ = r.Read(buf)
		_ = r.Close()

		select {
		case code := <-done:
			if code != 0 {
				t.Errorf("expected exit code 0 on pipe close, got %d", code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("yes did not terminate after pipe close")
		}
	})

	t.Run("context cancellation terminates cleanly", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		var stderr bytes.Buffer
		ioEnv := core.IO{Stdout: io.Discard, Stderr: &stderr}

		done := make(chan int, 1)
		go func() {
			done <- Main(ctx, ioEnv)
		}()

		select {
		case code := <-done:
			if code != 130 {
				t.Errorf("expected exit code 130 on ctx cancel, got %d", code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("yes did not terminate after ctx cancellation")
		}
	})

	t.Run("help flag returns 0", func(t *testing.T) {
		var stderr bytes.Buffer
		ioEnv := core.IO{Stdout: io.Discard, Stderr: &stderr}
		code := Main(context.Background(), ioEnv, "-help")
		if code != 0 {
			t.Fatalf("expected 0 on -help, got %d", code)
		}
	})
}
