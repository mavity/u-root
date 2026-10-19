// Copyright 2012-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// gzip compresses files using gzip compression.
package gzipcmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	pkggzip "github.com/u-root/u-root/pkg/gzip"
)

func outputPath(path string, opts *pkggzip.Options) string {
	if opts.Stdout || opts.Test {
		return path
	} else if opts.Decompress {
		return strings.TrimSuffix(path, opts.Suffix)
	}
	return path + opts.Suffix
}

func checkPath(resolvedPath string, opts *pkggzip.Options) error {
	if opts.Stdin {
		return nil
	}

	_, err := os.Stat(resolvedPath)
	if os.IsNotExist(err) || os.IsPermission(err) {
		return err
	}

	if !opts.Force {
		if opts.Decompress {
			if !strings.HasSuffix(resolvedPath, opts.Suffix) {
				return fmt.Errorf("%q does not have %q suffix", resolvedPath, opts.Suffix)
			}
		} else {
			if strings.HasSuffix(resolvedPath, opts.Suffix) {
				return fmt.Errorf("%q already has %q suffix", resolvedPath, opts.Suffix)
			}
		}
	}
	return nil
}

func checkOutputPath(resolvedOut string, opts *pkggzip.Options) error {
	_, err := os.Stat(resolvedOut)
	if !os.IsNotExist(err) && !opts.Stdout && !opts.Test && !opts.Force {
		return err
	} else if os.IsPermission(err) {
		return err
	}
	return nil
}

func processFile(ctx context.Context, ioEnv core.IO, resolvedPath, resolvedOut string, opts *pkggzip.Options) error {
	var inReader io.Reader
	if opts.Stdin {
		inReader = ioEnv.Stdin
	} else {
		f, err := os.Open(resolvedPath)
		if err != nil {
			return err
		}
		defer f.Close()
		inReader = f
	}

	var outWriter io.Writer
	var outCloser io.Closer
	if opts.Test {
		outWriter = io.Discard
	} else if opts.Stdout {
		outWriter = ioEnv.Stdout
	} else {
		f, err := os.Create(resolvedOut)
		if err != nil {
			return err
		}
		outWriter = f
		outCloser = f
	}

	if opts.Verbose && !opts.Quiet {
		inName := "stdin"
		if !opts.Stdin {
			inName = resolvedPath
		}
		outName := "stdout"
		if !opts.Stdout && !opts.Test {
			outName = resolvedOut
		}
		fmt.Fprintf(ioEnv.Stderr, "%s to %s\n", inName, outName)
	}

	var opErr error
	if opts.Decompress {
		opErr = pkggzip.Decompress(inReader, outWriter, opts.Blocksize, opts.Processes)
	} else {
		opErr = pkggzip.Compress(inReader, outWriter, opts.Level, opts.Blocksize, opts.Processes)
	}

	if outCloser != nil {
		if closeErr := outCloser.Close(); closeErr != nil && opErr == nil {
			opErr = closeErr
		}
	}
	return opErr
}

func run(ctx context.Context, ioEnv core.IO, progName string, args ...string) error {
	fs := flag.NewFlagSet(progName, flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(ioEnv.Stderr, "Usage of %s:\n", progName)
		fs.PrintDefaults()
	}

	var opts pkggzip.Options
	if err := opts.ParseArgs(progName, args, fs); err != nil {
		if errors.Is(err, pkggzip.ErrHelp) {
			fs.Usage()
			return nil
		}
		return err
	}

	var fileArgs []string
	if len(fs.Args()) == 0 {
		opts.Stdin = true
		opts.Stdout = true
		return processFile(ctx, ioEnv, "", "", &opts)
	}
	fileArgs = fs.Args()

	for _, rawPath := range fileArgs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resolvedPath := ioEnv.ResolvePath(rawPath)
		if err := checkPath(resolvedPath, &opts); err != nil {
			if !opts.Quiet {
				fmt.Fprintf(ioEnv.Stderr, "%s\n", err)
			}
			continue
		}

		outP := outputPath(rawPath, &opts)
		resolvedOut := ioEnv.ResolvePath(outP)
		if err := checkOutputPath(resolvedOut, &opts); err != nil {
			if !opts.Quiet {
				fmt.Fprintf(ioEnv.Stderr, "%s\n", err)
			}
			continue
		}

		if err := processFile(ctx, ioEnv, resolvedPath, resolvedOut, &opts); err != nil {
			if !opts.Quiet {
				fmt.Fprintf(ioEnv.Stderr, "%s\n", err)
			}
			continue
		}

		if !opts.Keep && !opts.Stdout && !opts.Test {
			_ = os.Remove(resolvedPath)
		}
	}
	return nil
}

// Main runs the gzip command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "gzip: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, "gzip", args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "gzip: %v\n", err)
		}
		return 1
	}
	return 0
}

// GzcatMain runs gzip in decompression mode with output to stdout.
func GzcatMain(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "gzcat: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, "gzcat", args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "gzcat: %v\n", err)
		}
		return 1
	}
	return 0
}
