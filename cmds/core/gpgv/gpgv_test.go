// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package gpgv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestRunGPGV(t *testing.T) {
	for _, tt := range []struct {
		name     string
		keyfile  string
		sigfile  string
		datafile string
		wantErr  error
	}{
		{
			name:     "SuccessSignature",
			keyfile:  "testdata/key.pub",
			sigfile:  "testdata/datafile.sig",
			datafile: "testdata/datafile.txt",
		},
		{
			name:     "UsageErrorMissingDatafile",
			keyfile:  "testdata/key.pub",
			sigfile:  "testdata/datafile.sig",
			datafile: "",
			wantErr:  errUsage,
		},
		{
			name:     "UsageErrorMissingSigFile",
			keyfile:  "testdata/key.pub",
			sigfile:  "",
			datafile: "testdata/datafile.txt",
			wantErr:  errUsage,
		},
		{
			name:     "UsageErrorMissingKeyFile",
			keyfile:  "",
			sigfile:  "testdata/datafile.sig",
			datafile: "testdata/datafile.txt",
			wantErr:  errUsage,
		},
		{
			name:     "UsageError",
			keyfile:  "",
			sigfile:  "",
			datafile: "",
			wantErr:  errUsage,
		},
		{
			name:     "InvalidKeyFile",
			keyfile:  "testdata/datafile.txt",
			sigfile:  "testdata/datafile.sig",
			datafile: "testdata/datafile.txt",
			wantErr:  fmt.Errorf("tag byte does not have MSB set"),
		},
		{
			name:     "InvalidKeyFilePrivateKey",
			keyfile:  "testdata/private.key",
			sigfile:  "testdata/datafile.sig",
			datafile: "testdata/datafile.txt",
			wantErr:  errExpectedPacket,
		},
		{
			name:     "InvalidSignatureFile",
			keyfile:  "testdata/key.pub",
			sigfile:  "testdata/datafile.txt",
			datafile: "testdata/datafile.txt",
			wantErr:  fmt.Errorf("tag byte does not have MSB set"),
		},
		{
			name:     "KeyFileIsNotExists",
			keyfile:  "testdata/missing",
			sigfile:  "testdata/datafile.sig",
			datafile: "testdata/datafile.txt",
			wantErr:  os.ErrNotExist,
		},
		{
			name:     "SigFileIsNotExists",
			keyfile:  "testdata/key.pub",
			sigfile:  "testdata/missing",
			datafile: "testdata/datafile.txt",
			wantErr:  os.ErrNotExist,
		},
		{
			name:     "DataFileIsNotExists",
			keyfile:  "testdata/key.pub",
			sigfile:  "testdata/datafile.sig",
			datafile: "testdata/missing",
			wantErr:  os.ErrNotExist,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			ioEnv := core.IO{
				Stdout: &buf,
				Stderr: &buf,
			}
			var args []string
			if tt.keyfile != "" {
				args = append(args, tt.keyfile)
			}
			if tt.sigfile != "" {
				args = append(args, tt.sigfile)
			}
			if tt.datafile != "" {
				args = append(args, tt.datafile)
			}

			if err := run(context.Background(), ioEnv, args...); !errors.Is(err, tt.wantErr) {
				if err == nil || tt.wantErr == nil || !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("run(%v):= %q, want %q", args, err, tt.wantErr)
				}
			}
		})
	}
}
