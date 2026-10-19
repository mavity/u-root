// Copyright 2013-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Print process information.
//
// Synopsis:
//
//	ps [-Aaex] [aux]
package ps

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var (
	all     bool
	every   bool
	x       bool
	nSidTty bool
	aux     = false
)

// Flags represents the options for a ps run
type Flags struct {
	All     bool
	Every   bool
	X       bool
	NSidTty bool
	Aux     bool
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "ps: ps [flags] [aux]\n")
}

func runPS(w io.Writer, f Flags, args ...string) error {
	for _, a := range args {
		switch a {
		case "aux":
			f.All = true
			f.Every = true
			f.Aux = true
			f.X = true
		default:
			usage(w)
			return nil
		}
	}

	pT := NewProcessTable()
	if err := pT.LoadTable(); err != nil {
		return err
	}
	if pT.Len() == 0 {
		return nil
	}

	sort.Sort(pT)

	switch {
	case f.Aux:
		pT.headers = []string{"PID", "PGRP", "SID", "TTY", "STAT", "TIME", "COMMAND"}
		pT.fields = []string{"Pid", "Pgrp", "Sid", "Ctty", "State", "Time", "Cmd"}
	case f.X:
		pT.headers = []string{"PID", "TTY", "STAT", "TIME", "COMMAND"}
		pT.fields = []string{"Pid", "Ctty", "State", "Time", "Cmd"}
	default:
		pT.headers = []string{"PID", "TTY", "TIME", "CMD"}
		pT.fields = []string{"Pid", "Ctty", "Time", "Cmd"}
	}

	pT.PrepareString()
	pT.PrintHeader(w)

	eUID := os.Geteuid()

	for index, p := range pT.table {
		switch {
		case f.NSidTty:
			if p.Sid == p.Pid || p.Ctty == "?" {
				continue
			}
		case f.X:
			if eUID != p.uid && eUID != 0 {
				continue
			}
		case f.All || f.Every || f.Aux:
			// print all
		default:
			if pT.mProc != nil && p.Sid != pT.mProc.Sid && p.Pid != pT.mProc.Pid {
				if p.Sid != "0" {
					continue
				}
			}
		}

		pT.PrintProcess(index, w)
	}

	return nil
}

// Backward-compatible ps function for tests
func ps(w io.Writer, args ...string) error {
	f := Flags{
		All:     all,
		Every:   every,
		X:       x,
		NSidTty: nSidTty,
		Aux:     aux,
	}
	return runPS(w, f, args...)
}

// Main runs the ps command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "ps: panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("ps", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var f Flags
	fs.BoolVar(&f.All, "All", false, "Select all processes. Identical to -e.")
	fs.BoolVar(&f.All, "A", false, "Select all processes. Identical to -e. (shorthand)")
	fs.BoolVar(&f.Every, "every", false, "Select all processes. Identical to -A.")
	fs.BoolVar(&f.Every, "e", false, "Select all processes. Identical to -A. (shorthand)")
	fs.BoolVar(&f.X, "bsd", false, "BSD-Like style, with STAT Column and long CommandLine")
	fs.BoolVar(&f.X, "x", false, "BSD-Like style, with STAT Column and long CommandLine (shorthand)")
	fs.BoolVar(&f.NSidTty, "anSIDTTY", false, "Print all process except whose are session leaders or unlinked with terminal")
	fs.BoolVar(&f.NSidTty, "a", false, "Print all process except whose are session leaders or unlinked with terminal (shorthand)")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if err := runPS(ioEnv.Stdout, f, fs.Args()...); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "ps: %v\n", err)
		return 1
	}
	return 0
}
