// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shasum

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestSHASum(t *testing.T) {
	// Creating tmp files with data to hash
	tmpdir := t.TempDir()
	file1Path := filepath.Join(tmpdir, "file1")
	if err := os.WriteFile(file1Path, []byte("abcdef\n"), 0o644); err != nil {
		t.Fatalf("failed to write string to file1: %v", err)
	}

	file2Path := filepath.Join(tmpdir, "file2")
	if err := os.WriteFile(file2Path, []byte("pqra\n"), 0o644); err != nil {
		t.Fatalf("failed to write string to file2: %v", err)
	}

	for _, tt := range []struct {
		name string
		args []string
		want string
		err  error
	}{
		{
			name: "bufIn as input with sha1 sum",
			args: []string{},
			want: "bdc37c074ec4ee6050d68bc133c6b912f36474df -\n",
		},
		{
			name: "bufIn as input with sha256 sum",
			args: []string{"-a", "256"},
			want: "ae0666f161fed1a5dde998bbd0e140550d2da0db27db1d0e31e370f2bd366a57 -\n",
		},
		{
			name: "bufIn as input with sha512 sum",
			args: []string{"-a", "512"},
			want: "624eb88c6f2be3e77b1306f976bf1fb7b48855701d3ed2198a15f38bb12d76d26e8eefe6457bc036a3f93f28dd05512f5a399a319d48a58c38c590e182fe8159 -\n",
		},
		{
			name: "wrong path file",
			args: []string{"testfile"},
			err:  os.ErrNotExist,
		},
		{
			name: "file1 as input with invalid algorithm",
			args: []string{"-a", "42", file1Path},
			err:  os.ErrInvalid,
		},
		{
			name: "stdin as input with invalid algorithm",
			args: []string{"-a", "42"},
			err:  os.ErrInvalid,
		},
		{
			name: "file1 as input with sha224 sum",
			args: []string{"-a", "224", file1Path},
			want: fmt.Sprintf("%s %s\n", "9ea13b9e78c0502303d373c59e50b9caa6e64127291462867fda9201", file1Path),
		},
		{
			name: "file1 as input with sha384 sum",
			args: []string{"-a", "384", file1Path},
			want: fmt.Sprintf("%s %s\n", "3acd5e39ab549fdb0ab0b76d6f21f920ad65df76127a85734722af2206dcd82ecc0c8fffb431db7b2a6738facc839056", file1Path),
		},
		{
			name: "file1 as input with sha512224 sum",
			args: []string{"-a", "512224", file1Path},
			want: fmt.Sprintf("%s %s\n", "690dab7163b614fc196932f5e393f9e158ef58469759bd3d19154ea4", file1Path),
		},
		{
			name: "file1 as input with sha512256 sum",
			args: []string{"-a", "512256", file1Path},
			want: fmt.Sprintf("%s %s\n", "468deccd9458c343c921515da6b72bc38ca3491302e509b5db2dc15849227372", file1Path),
		},
		{
			name: "file1 as input with sha1 sum",
			args: []string{"-a", "1", file1Path},
			want: fmt.Sprintf("%s %s\n", "bdc37c074ec4ee6050d68bc133c6b912f36474df", file1Path),
		},
		{
			name: "file2 as input with sha1 sum",
			args: []string{"-a", "1", file2Path},
			want: fmt.Sprintf("%s %s\n", "e8ed2d487f1dc32152c8590f39c20b7703f9e159", file2Path),
		},
		{
			name: "file1 as input with sha256 sum",
			args: []string{"--algorithm", "256", file1Path},
			want: fmt.Sprintf("%s %s\n", "ae0666f161fed1a5dde998bbd0e140550d2da0db27db1d0e31e370f2bd366a57", file1Path),
		},
		{
			name: "file2 as input with sha256 sum",
			args: []string{"-a", "256", file2Path},
			want: fmt.Sprintf("%s %s\n", "db296dd0bcb796df9b327f44104029da142c8fff313a25bd1ac7c3b7562caea9", file2Path),
		},
		{
			name: "file1 and file 2 as input with sha256 sum",
			args: []string{"-a", "256", file1Path, file2Path},
			want: fmt.Sprintf("%s %s\n%s %s\n", "ae0666f161fed1a5dde998bbd0e140550d2da0db27db1d0e31e370f2bd366a57", file1Path,
				"db296dd0bcb796df9b327f44104029da142c8fff313a25bd1ac7c3b7562caea9", file2Path),
		},
		{
			name: "file1 as input with sha512 sum",
			args: []string{"-a", "512", file1Path},
			want: fmt.Sprintf("%s %s\n", "624eb88c6f2be3e77b1306f976bf1fb7b48855701d3ed2198a15f38bb12d76d26e8eefe6457bc036a3f93f28dd05512f5a399a319d48a58c38c590e182fe8159", file1Path),
		},
		{
			name: "file2 as input with sha512 sum",
			args: []string{"-a", "512", file2Path},
			want: fmt.Sprintf("%s %s\n", "53eb6dc4fc160a443941c53b40cc1d08b212b140c8a5030bb3c035e184c74898155ab811aafde46f8f4c0989fe49ac6fd72fb13bafe21b1ea32a452bf3a01c6d", file2Path),
		},
		{
			name: "file1 and file 2 as input with sha512 sum",
			args: []string{"-a", "512", file1Path, file2Path},
			want: fmt.Sprintf("%s %s\n%s %s\n",
				"624eb88c6f2be3e77b1306f976bf1fb7b48855701d3ed2198a15f38bb12d76d26e8eefe6457bc036a3f93f28dd05512f5a399a319d48a58c38c590e182fe8159", file1Path,
				"53eb6dc4fc160a443941c53b40cc1d08b212b140c8a5030bb3c035e184c74898155ab811aafde46f8f4c0989fe49ac6fd72fb13bafe21b1ea32a452bf3a01c6d", file2Path),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bufIn := bytes.NewBufferString("abcdef\n")
			bufOut := &bytes.Buffer{}
			ioEnv := core.IO{Stdin: bufIn, Stdout: bufOut, Stderr: io.Discard}

			got := run(context.Background(), ioEnv, tt.args...)
			if got != nil {
				if tt.err != nil && errors.Is(got, tt.err) {
					return
				}
				t.Errorf("shasum() = %q, want: %q", got, tt.err)
			} else {
				if bufOut.String() != tt.want {
					t.Errorf("shasum() = %q, want: %q", bufOut.String(), tt.want)
				}
			}
		})
	}
}
