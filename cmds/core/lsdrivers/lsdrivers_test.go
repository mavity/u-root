// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lsdrivers

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

type dfl struct {
	t string
	n string
	l string
}

func makeDFL(d []dfl) error {
	for _, f := range d {
		switch f.t {
		case "d":
			if err := os.MkdirAll(f.n, 0o755); err != nil {
				return err
			}
		case "l":
			if err := os.Symlink(f.l, f.n); err != nil {
				return err
			}
		case "-":
			f, err := os.Create(f.n)
			if err != nil {
				return err
			}
			f.Close()
		}
	}
	return nil
}

func allDone(dir string, t *testing.T) {
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
}

func TestUnusedAlarmTimer(t *testing.T) {
	dir := t.TempDir()

	alarmtimer := []dfl{
		{t: "d", n: filepath.Join(dir, "sys/bus/platform/drivers/alarmtimer/"), l: ""},
	}
	if err := makeDFL(alarmtimer); err != nil {
		t.Fatal(err)
	}
	defer allDone(dir, t)

	bus := filepath.Join(dir, "sys/bus")
	out, err := lsdrivers(bus, false)
	if err != nil {
		t.Fatal(err)
	}
	o := strings.Join(out, "\n")
	used := ""
	if used != o {
		t.Errorf("testing used: got %q, want %q", o, used)
	}
	out, err = lsdrivers(bus, true)
	if err != nil {
		t.Fatal(err)
	}
	unused := "platform.alarmtimer"
	o = strings.Join(out, "\n")
	if unused != o {
		t.Errorf("testing unused: got %q, want %q", o, unused)
	}
}

func TestUsedPWRButton(t *testing.T) {
	dir := t.TempDir()

	alarmtimer := []dfl{
		{t: "d", n: filepath.Join(dir, "sys/bus/acpi/drivers/button/"), l: ""},
		{t: "l", n: filepath.Join(dir, "sys/bus/acpi/drivers/button/LNXPWRBN:00"), l: "../../../../devices/LNXSYSTM:00/LNXPWRBN:00"},
	}
	if err := makeDFL(alarmtimer); err != nil {
		t.Skipf("skipping TestUsedPWRButton (symlink creation not supported): %v", err)
	}
	defer allDone(dir, t)

	bus := filepath.Join(dir, "sys/bus")
	out, err := lsdrivers(bus, false)
	if err != nil {
		t.Fatal(err)
	}
	o := strings.Join(out, "\n")
	used := "acpi.button"
	if used != o {
		t.Errorf("testing used: got %q, want %q", o, used)
	}
	out, err = lsdrivers(bus, true)
	if err != nil {
		t.Fatal(err)
	}
	unused := ""
	o = strings.Join(out, "\n")
	if unused != o {
		t.Errorf("testing unused: got %q, want %q", o, unused)
	}
}

func TestMain(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ioEnv := core.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr}

	// 1. lsdrivers -help -> exit code 0
	code := Main(context.Background(), ioEnv, "-help")
	if code != 0 {
		t.Fatalf("Main(-help) returned %d, want 0", code)
	}
}
