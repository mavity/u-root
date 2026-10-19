// Copyright 2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package timeout

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/u-root/u-root/pkg/core"
)

func TestBadInvocation(t *testing.T) {
	tests := []struct {
		cmd   cmd
		err   error
		errno int
	}{
		{cmd: cmd{args: []string{}, signal: "KILL"}, err: errNoArgs, errno: 1},
		{cmd: cmd{args: []string{"echo", "WOO"}, signal: "WOO"}, err: os.ErrInvalid, errno: 1},
	}

	for _, v := range tests {
		t.Logf("Run %v", v.cmd)
		if errno, err := v.cmd.run(); !errors.Is(err, v.err) || errno != v.errno {
			t.Errorf("run %v: got (%d, %v), want (%d, %v)", v.cmd, errno, err, v.errno, v.err)
		}
	}
}

func TestTimeoutMain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ioEnv := core.IO{
		Stdout: &stdout,
		Stderr: &stderr,
		Runner: func(ctx context.Context, subIO core.IO, args ...string) int {
			select {
			case <-time.After(200 * time.Millisecond):
				return 0
			case <-ctx.Done():
				return 130
			}
		},
	}

	// Should succeed when timeout is longer than runner duration
	code := Main(context.Background(), ioEnv, "-t", "1s", "sleepy")
	if code != 0 {
		t.Fatalf("expected 0, got %d, stderr: %s", code, stderr.String())
	}

	// Should timeout when timeout is shorter than runner duration
	stdout.Reset()
	stderr.Reset()
	code = Main(context.Background(), ioEnv, "-t", "50ms", "sleepy")
	if code != 124 {
		t.Fatalf("expected 124 on timeout, got %d", code)
	}
}
