// Copyright 2015-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Print build information about the kernel and machine.
//
// Synopsis:
//
//	uname [-asnrvmd]
//
// Options:
//
//	-a: print everything
//	-s: print the kernel name
//	-n: print the network node name
//	-r: print the kernel release
//	-v: print the kernel version
//	-m: print the machine hardware name
//	-p: print the machine hardware name
package uname

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type utsname struct {
	Sysname  string
	Nodename string
	Release  string
	Version  string
	Machine  string
}

type params struct {
	kernel  bool
	node    bool
	release bool
	version bool
	machine bool
}

func handleFlags(u *utsname, p params) string {
	info := make([]string, 0, 5)

	if p.kernel {
		info = append(info, u.Sysname)
	}
	if p.node {
		info = append(info, u.Nodename)
	}
	if p.release {
		info = append(info, u.Release)
	}
	if p.version {
		info = append(info, u.Version)
	}
	if p.machine {
		info = append(info, u.Machine)
	}

	return strings.Join(info, " ")
}

func run(stdout io.Writer, p params) error {
	u, err := getUtsname()
	if err != nil {
		return err
	}
	info := handleFlags(&u, p)
	_, err = fmt.Fprintln(stdout, info)
	return err
}

func parseParams(all, kernel, node, release, version, machine, processor bool) params {
	p := params{
		kernel:  kernel || all,
		node:    node || all,
		release: release || all,
		version: version || all,
		machine: machine || processor || all,
	}

	if !p.kernel && !p.node && !p.release && !p.version && !p.machine {
		p.kernel = true
	}

	return p
}

// Main executes the uname command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "uname: panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("uname", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	all := fs.Bool("a", false, "print everything")
	kernel := fs.Bool("s", false, "print the kernel name")
	node := fs.Bool("n", false, "print the network node name")
	release := fs.Bool("r", false, "print the kernel release")
	version := fs.Bool("v", false, "print the kernel version")
	machine := fs.Bool("m", false, "print the machine hardware name")
	processor := fs.Bool("p", false, "print the machine hardware name")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	p := parseParams(*all, *kernel, *node, *release, *version, *machine, *processor)
	if err := run(ioEnv.Stdout, p); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "uname: %v\n", err)
		return 1
	}
	return 0
}
