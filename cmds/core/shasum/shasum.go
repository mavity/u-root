// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// shasum computes SHA checksums of files.
//
// Synopsis:
//
//	shasum -a <algorithm> <File Name>
//
// Description:
//
//	shasum computes SHA checksums of files using the specified algorithm.
//	If no files are specified, read from stdin.
//
// Options:
//
//	-a, -algorithm: SHA algorithm, valid args are 1, 224, 256, 384, 512, 512224 and 512256
package shasum

import (
	"bufio"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

// shaGenerator generates SHA hash of given data.
func shaGenerator(r io.Reader, algo int) ([]byte, error) {
	var h hash.Hash
	switch algo {
	case 1:
		h = sha1.New()
	case 224:
		h = sha256.New224()
	case 256:
		h = sha256.New()
	case 384:
		h = sha512.New384()
	case 512:
		h = sha512.New()
	case 512224:
		h = sha512.New512_224()
	case 512256:
		h = sha512.New512_256()
	default:
		return nil, fmt.Errorf("invalid algorithm, only 1, 224, 256, 384, 512, 512224 and 512256 are valid: %w", os.ErrInvalid)
	}
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	f := flag.NewFlagSet("shasum", flag.ContinueOnError)
	f.SetOutput(ioEnv.Stderr)

	var algorithm int
	f.IntVar(&algorithm, "algorithm", 1, "SHA algorithm, valid args are 1, 224, 256, 384, 512, 512224 and 512256")
	f.IntVar(&algorithm, "a", 1, "SHA algorithm, valid args are 1, 224, 256, 384, 512, 512224 and 512256")

	f.Usage = func() {
		fmt.Fprintf(f.Output(), "%s\n", "Usage:\nshasum -a <algorithm> <File Name>")
		f.PrintDefaults()
	}

	if err := f.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}
	fileArgs := f.Args()

	var hashbytes []byte
	var err error
	if len(fileArgs) == 0 {
		buf := bufio.NewReader(ioEnv.Stdin)
		if hashbytes, err = shaGenerator(buf, algorithm); err != nil {
			return err
		}
		fmt.Fprintf(ioEnv.Stdout, "%x -\n", hashbytes)
		return nil
	}
	for _, arg := range fileArgs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resolved := ioEnv.ResolvePath(arg)
		file, err := os.Open(resolved)
		if err != nil {
			return err
		}
		hashbytes, err = shaGenerator(file, algorithm)
		_ = file.Close()
		if err != nil {
			return err
		}
		fmt.Fprintf(ioEnv.Stdout, "%x %s\n", hashbytes, arg)
	}
	return nil
}

// Main runs the shasum command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "shasum: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "shasum: %v\n", err)
		}
		return 1
	}
	return 0
}
