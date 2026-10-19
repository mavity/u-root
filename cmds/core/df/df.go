// Copyright 2015-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// df reports details of mounted filesystems.
//
// Synopsis
//
//	df [-k] [-m] [file...]
//
// Options
//
//	-k: display values in KB (default)
//	-m: display values in MB
package df

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type flags struct {
	k bool
	m bool
}

const (
	B  uint64 = 1
	KB uint64 = 1024 * B
	MB uint64 = 1024 * KB
)

var (
	errKMExclusiv = errors.New("options -k and -m are mutually exclusive")
)

// Mount is a structure used to contain mount point data
type Mount struct {
	Device         string
	MountPoint     string
	FileSystemType string
	Flags          string
	Bsize          int64
	Blocks         uint64
	Total          uint64
	Used           uint64
	Avail          uint64
	PCT            uint8
}

// setUnits returns the correct units used to calculate display values
func setUnits(inKB, inMB bool) (uint64, error) {
	if inKB && inMB {
		return 0, errKMExclusiv
	}
	if inMB {
		return MB, nil
	}
	return KB, nil
}

func printHeader(w io.Writer, blockSize string) {
	fmt.Fprintf(w, "Filesystem           Type         %v-blocks       Used    Available  Use%% Mounted on\n", blockSize)
}

func printMount(w io.Writer, mnt Mount) {
	fmt.Fprintf(w, "%-20v %-9v %12v %10v %12v %4v%% %-13v\n",
		mnt.Device,
		mnt.FileSystemType,
		mnt.Blocks,
		mnt.Used,
		mnt.Avail,
		mnt.PCT,
		mnt.MountPoint)
}

func df(w io.Writer, fargs flags, args []string) error {
	units, err := setUnits(fargs.k, fargs.m)
	if err != nil {
		return err
	}
	mounts, err := getMounts(units)
	if err != nil {
		return fmt.Errorf("mountinfo: %w", err)
	}
	blocksize := "1K"
	if fargs.m {
		blocksize = "1M"
	}

	if len(args) == 0 {
		printHeader(w, blocksize)
		for _, mnt := range mounts {
			printMount(w, mnt)
		}
		return nil
	}

	var fileDevs []uint64
	for _, arg := range args {
		fileDev, err := deviceNumber(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "df: %v\n", err)
			continue
		}
		fileDevs = append(fileDevs, fileDev)
	}

	showHeader := true
	for _, mnt := range mounts {
		stDev, err := deviceNumber(mnt.MountPoint)
		if err != nil {
			fmt.Fprintf(os.Stderr, "df: %v\n", err)
			continue
		}

		for _, fDev := range fileDevs {
			if fDev == stDev {
				if showHeader {
					printHeader(w, blocksize)
					showHeader = false
				}
				printMount(w, mnt)
			}
		}
	}

	return nil
}

// Main runs the df command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "df: panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("df", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var fargs flags
	fs.BoolVar(&fargs.k, "k", false, "Express the values in kilobytes (default)")
	fs.BoolVar(&fargs.m, "m", false, "Express the values in megabytes")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	resolvedArgs := make([]string, len(fs.Args()))
	for i, a := range fs.Args() {
		resolvedArgs[i] = ioEnv.ResolvePath(a)
	}

	if err := df(ioEnv.Stdout, fargs, resolvedArgs); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "df: %v\n", err)
		return 1
	}
	return 0
}
