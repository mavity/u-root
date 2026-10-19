// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Scp copies files between hosts on a network.
//
// Synopsis:
//
//	scp [-t|-f] [FILE]
//
// Description:
//
//	If -t is given, decode SCP protocol from stdin and write to FILE.
//	If -f is given, stream FILE over SCP protocol to stdout.
//
// Options:
//
//	-t: Act as the target
//	-f: Act as the source
//	-v: Passed if SCP is verbose, ignored
package scp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/u-root/u-root/pkg/core"
)

const (
	SUCCESS = 0
)

func scpSingleSource(w io.Writer, r io.Reader, pth string) error {
	f, err := os.Open(pth)
	if err != nil {
		return err
	}
	defer f.Close()
	s, err := f.Stat()
	if err != nil {
		return err
	}
	filename := filepath.Base(pth)
	fmt.Fprintf(w, "C0%o %d %s\n", s.Mode().Perm(), s.Size(), filename)
	if response(r) != SUCCESS {
		return fmt.Errorf("response was not success")
	}
	_, err = io.Copy(w, f)
	if err != nil {
		return fmt.Errorf("copy error: %w", err)
	}
	reply(w, SUCCESS)

	if response(r) != SUCCESS {
		return fmt.Errorf("response was not success")
	}
	return nil
}

func scpSingleSink(w io.Writer, r io.Reader, path string) error {
	var mode os.FileMode
	var size int64
	filename := ""

	// Ignore the filename, assume it has been provided on the command line.
	// This will not work with directories and recursive copy, but that's not
	// supported right now.
	if _, err := fmt.Fscanf(r, "C0%o %d %s\n", &mode, &size, &filename); err != nil {
		if err == io.ErrUnexpectedEOF {
			return io.EOF
		}
		return fmt.Errorf("fscanf: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open error: %w", err)
	}
	reply(w, SUCCESS)
	defer f.Close()

	_, err = io.CopyN(f, r, size)
	if err != nil {
		return fmt.Errorf("copy error: %w", err)
	}
	if response(r) != SUCCESS {
		return fmt.Errorf("response was not success")
	}
	reply(w, SUCCESS)
	return nil
}

func scpSource(w io.Writer, r io.Reader, path string) error {
	// Sink->Source is started with a response
	if response(r) != SUCCESS {
		return fmt.Errorf("response was not success")
	}
	return scpSingleSource(w, r, path)
}

func scpSink(w io.Writer, r io.Reader, path string) error {
	reply(w, SUCCESS)
	for {
		if err := scpSingleSink(w, r, path); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	return nil
}

func reply(out io.Writer, r byte) {
	out.Write([]byte{r})
}

func response(in io.Reader) byte {
	b := make([]byte, 1)
	in.Read(b)
	return b[0]
}

// Main runs scp with the given context, I/O environment, and arguments.
func Main(ctx context.Context, ioEnv core.IO, args ...string) int {
	if err := runWithIO(ctx, ioEnv, args...); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(ioEnv.Stderr, "scp: %v\n", err)
		return 1
	}
	return 0
}

func runWithIO(ctx context.Context, ioEnv core.IO, args ...string) error {
	fs := flag.NewFlagSet("scp", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var isTarget, isSource bool
	var ignored bool
	fs.BoolVar(&isTarget, "t", false, "Act as the target")
	fs.BoolVar(&isSource, "f", false, "Act as the source")
	fs.BoolVar(&ignored, "v", false, "Ignored")

	parseArgs := args
	if len(parseArgs) > 0 && !strings.HasPrefix(parseArgs[0], "-") {
		if parseArgs[0] == "scp" || filepath.Base(parseArgs[0]) == "scp" {
			parseArgs = parseArgs[1:]
		}
	}

	if err := fs.Parse(parseArgs); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("no file provided")
	}

	if isSource == isTarget {
		return fmt.Errorf("-t or -f needs to be supplied, and not both")
	}

	targetPath := ioEnv.ResolvePath(fs.Arg(0))

	if isSource {
		return scpSource(ioEnv.Stdout, ioEnv.Stdin, targetPath)
	}
	return scpSink(ioEnv.Stdout, ioEnv.Stdin, targetPath)
}
