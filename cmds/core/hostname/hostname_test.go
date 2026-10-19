// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hostname

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestHostname(t *testing.T) {
	t.Run("wrong number of args", func(t *testing.T) {
		var stderr bytes.Buffer
		ioEnv := core.IO{Stderr: &stderr}
		code := Main(context.Background(), ioEnv, "b", "c")
		if code == 0 {
			t.Error("expected exit code != 0, got 0")
		}
	})

	t.Run("should return hostname", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{Stdout: &stdout, Stderr: &stderr}
		code := Main(context.Background(), ioEnv)
		if code != 0 {
			t.Errorf("expected 0, got %d, stderr: %s", code, stderr.String())
		}
		if strings.TrimSpace(stdout.String()) == "" {
			t.Error("expect non empty hostname")
		}
	})

	t.Run("respects HOSTNAME env", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{
			Stdout: &stdout,
			Stderr: &stderr,
			LookupEnv: func(k string) (string, bool) {
				if k == "HOSTNAME" {
					return "virtual-box.localdomain", true
				}
				return "", false
			},
		}
		code := Main(context.Background(), ioEnv)
		if code != 0 || strings.TrimSpace(stdout.String()) != "virtual-box.localdomain" {
			t.Fatalf("expected virtual-box.localdomain, got code=%d out=%q", code, stdout.String())
		}
	})

	t.Run("short flag -s parses as flag without destructive Sethostname", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{
			Stdout: &stdout,
			Stderr: &stderr,
			LookupEnv: func(k string) (string, bool) {
				if k == "HOSTNAME" {
					return "node1.cluster.internal", true
				}
				return "", false
			},
		}
		code := Main(context.Background(), ioEnv, "-s")
		if code != 0 || strings.TrimSpace(stdout.String()) != "node1" {
			t.Fatalf("expected short hostname node1, got code=%d out=%q", code, stdout.String())
		}
	})

	t.Run("domain flag -d", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ioEnv := core.IO{
			Stdout: &stdout,
			Stderr: &stderr,
			LookupEnv: func(k string) (string, bool) {
				if k == "HOSTNAME" {
					return "node1.cluster.internal", true
				}
				return "", false
			},
		}
		code := Main(context.Background(), ioEnv, "-d")
		if code != 0 || strings.TrimSpace(stdout.String()) != "cluster.internal" {
			t.Fatalf("expected domain cluster.internal, got code=%d out=%q", code, stdout.String())
		}
	})

	t.Run("from file flag -F", func(t *testing.T) {
		dir := t.TempDir()
		hostFile := filepath.Join(dir, "myhost.txt")
		if err := os.WriteFile(hostFile, []byte("# comment\nnewbox.example.com\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		var setEnvKey, setEnvVal string
		ioEnv := core.IO{
			Stdout: &stdout,
			Stderr: &stderr,
			Setenv: func(k, v string) error {
				setEnvKey = k
				setEnvVal = v
				return nil
			},
		}
		_ = Main(context.Background(), ioEnv, "-F", hostFile)
		if setEnvKey != "HOSTNAME" || setEnvVal != "newbox.example.com" {
			t.Errorf("expected Setenv HOSTNAME=newbox.example.com, got %s=%s", setEnvKey, setEnvVal)
		}
	})
}
