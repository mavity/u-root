// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// lsdrivers lists driver usage on Linux systems
//
// Synopsis:
//
//	lsdrivers [-u]
//
// Description:
//
//	List driver usage. This program is mostly useful for scripts.
//
// Options:
//
//	-u: list unused drivers
package lsdrivers

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/u-root/u-root/pkg/core"
)

// hasDevices returns true if there is a non-symlink, i.e. regular,
// file in driverpath. This indicates that it is real hardware.
// If the path has any sort of error, return false.
func hasDevices(driverpath string) bool {
	files, err := os.ReadDir(driverpath)
	if err != nil {
		return false
	}
	for _, file := range files {
		if file.Type()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func lsdrivers(bus string, unused bool) ([]string, error) {
	var d []string
	files, err := os.ReadDir(bus)
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		n := filepath.Join(bus, f.Name(), "drivers")
		drivers, err := os.ReadDir(n)
		// In some cases the directory does not exist.
		if err != nil {
			continue
		}
		for _, driver := range drivers {
			n := filepath.Join(bus, f.Name(), "drivers", driver.Name())
			if hasDevices(n) != unused {
				d = append(d, fmt.Sprintf("%s.%s", f.Name(), driver.Name()))
			}
		}
	}
	return d, nil
}

// Main executes lsdrivers reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "lsdrivers: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	_ = ctx
	fs := flag.NewFlagSet("lsdrivers", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var unused = fs.Bool("u", false, "Show unused drivers")

	fs.Usage = func() {
		fmt.Fprintln(ioEnv.Stderr, "Usage: lsdrivers [-u]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	drivers, err := lsdrivers("/sys/bus", *unused)
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "lsdrivers: %v\n", err)
		return 1
	}

	if len(drivers) > 0 {
		fmt.Fprintln(ioEnv.Stdout, strings.Join(drivers, "\n"))
	}
	return 0
}
