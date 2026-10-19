// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

func TestScpSource(t *testing.T) {
	var w bytes.Buffer
	var r bytes.Buffer

	tf, err := os.CreateTemp("", "TestScpSource")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	defer os.Remove(tf.Name())
	defer tf.Close()
	if _, err := tf.Write([]byte("test-file-contents")); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	st, err := tf.Stat()
	if err != nil {
		t.Fatalf("stat temp file: %v", err)
	}

	r.Write([]byte{0})
	err = scpSource(&w, &r, tf.Name())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	expected := fmt.Appendf(nil, "C0%o 18 %s\ntest-file-contents", st.Mode().Perm(), filepath.Base(tf.Name()))
	expected = append(expected, 0)
	if string(expected) != w.String() {
		t.Fatalf("Got: %v\nExpected: %v", w.String(), string(expected))
	}
}

func TestScpSink(t *testing.T) {
	var w bytes.Buffer
	var r bytes.Buffer

	tf, err := os.CreateTemp("", "TestScpSink")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	name := tf.Name()
	_ = tf.Close()
	defer os.Remove(name)

	r.Write([]byte("C0600 18 test\ntest-file-contents"))
	// Post IO-copy success status
	r.Write([]byte{0})

	err = scpSink(&w, &r, name)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// 1: Initial SUCCESS post to start scp
	// 2: Success opening file name
	// 3: Success writing file
	expected := []byte{0, 0, 0}
	if string(expected) != w.String() {
		t.Fatalf("Got: %v\nExpected: %v", w.Bytes(), expected)
	}

	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(content) != "test-file-contents" {
		t.Fatalf("Expected 'test-file-contents', got %q", string(content))
	}
}

func TestScpMain(t *testing.T) {
	ctx := context.Background()

	// Missing args
	var stderr bytes.Buffer
	code := Main(ctx, core.IO{Stderr: &stderr}, "-t")
	if code == 0 {
		t.Errorf("expected non-zero exit code for missing file arg")
	}

	// Both -t and -f provided
	stderr.Reset()
	code = Main(ctx, core.IO{Stderr: &stderr}, "-t", "-f", "foo.txt")
	if code == 0 {
		t.Errorf("expected non-zero exit code for both -t and -f")
	}

	// Neither -t nor -f provided
	stderr.Reset()
	code = Main(ctx, core.IO{Stderr: &stderr}, "foo.txt")
	if code == 0 {
		t.Errorf("expected non-zero exit code for neither -t nor -f")
	}
}
