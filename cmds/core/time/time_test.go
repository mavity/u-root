// Copyright 2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package timecmd

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestTime(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{
			name: "NoArgs",
			want: "real 0.000.*\nuser 0.000.*\nsys 0.000",
		},
		{
			name: "WithRunner",
			args: []string{"testcmd", "arg1"},
			want: "real [0-9][0-9]*.*\nuser 0.000.*\nsys 0.000",
		},
		{
			name:    "CommandNotFound",
			args:    []string{"deadbeef-nonexistent-command-xyz"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdin, stdout, stderr bytes.Buffer
			ioEnv := core.IO{
				Stdin:  &stdin,
				Stdout: &stdout,
				Stderr: &stderr,
			}
			if tt.name == "WithRunner" {
				ioEnv.Runner = func(ctx context.Context, subIO core.IO, args ...string) int {
					return 0
				}
			}

			err := run(context.Background(), ioEnv, tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("run() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			res := stderr.String()
			m, err := regexp.MatchString(tt.want, res)
			if err != nil {
				t.Fatal(err)
			}
			if !m {
				t.Errorf("regexp.MatchString(%q, %q) false, wanted match", tt.want, res)
			}
		})
	}
}
