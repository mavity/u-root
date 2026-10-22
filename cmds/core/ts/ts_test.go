// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ts

import (
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestTS(t *testing.T) {
	stdin := strings.NewReader("hello\nworld\n")
	stdout := &bytes.Buffer{}

	err := run(stdin, stdout, []string{"ts", "-R", "-f"})
	if err != nil {
		t.Error(err)
	}

	// ts format for relative option
	ts := `^\[\+([0-9]*\.)[0-9]{4}s]`

	lines := strings.Split(stdout.String(), "\n")
	l1 := lines[0]
	m, err := regexp.MatchString(ts+" hello$", l1)
	if !m || err != nil {
		t.Errorf("expected timestamped line, got %q", l1)
	}
	l2 := lines[1]
	m, err = regexp.MatchString(ts+" world$", l2)
	if !m || err != nil {
		t.Errorf("expected timestamped line, got %q", l2)
	}
}

func TestInvalidUse(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}

	err := run(stdin, stdout, []string{"ts", "foo"})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestMainWriterError(t *testing.T) {
	code := Main(context.Background(), core.IO{
		Stdin:  strings.NewReader("line\n"),
		Stdout: tsErrorWriter{err: errors.New("write failed")},
		Stderr: io.Discard,
	})
	if code == 0 {
		t.Fatal("ts returned success after output failure")
	}
}

type tsErrorWriter struct {
	err error
}

func (w tsErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
