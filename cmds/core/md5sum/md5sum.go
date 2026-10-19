// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// md5sum prints an md5 hash generated from file contents.
package md5sum

import (
	"bufio"
	"context"
	"crypto/md5"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
	"github.com/u-root/u-root/pkg/uroot/util"
)

var usage = "md5sum: md5sum [-c] [File Name...]"

func calculateMd5Sum(ctx context.Context, r io.Reader) ([]byte, error) {
	h := md5.New()
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		n, err := r.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	return h.Sum(nil), nil
}

func checkMd5File(ctx context.Context, ioEnv core.IO, listFile string) error {
	var r io.Reader
	if listFile == "-" {
		r = ioEnv.Stdin
	} else {
		f, err := os.Open(ioEnv.ResolvePath(listFile))
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	}

	scanner := bufio.NewScanner(r)
	failed := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		expected := strings.ToLower(fields[0])
		target := strings.TrimPrefix(fields[1], "*")
		tf, err := os.Open(ioEnv.ResolvePath(target))
		if err != nil {
			fmt.Fprintf(ioEnv.Stdout, "%s: FAILED open or read\n", target)
			failed++
			continue
		}
		sum, err := calculateMd5Sum(ctx, tf)
		_ = tf.Close()
		if err != nil || fmt.Sprintf("%x", sum) != expected {
			fmt.Fprintf(ioEnv.Stdout, "%s: FAILED\n", target)
			failed++
		} else {
			fmt.Fprintf(ioEnv.Stdout, "%s: OK\n", target)
		}
	}
	if failed > 0 {
		return fmt.Errorf("md5sum: WARNING: %d computed checksum(s) did NOT match", failed)
	}
	return scanner.Err()
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var check bool
	fs := flag.NewFlagSet("md5sum", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&check, "c", false, "read MD5 sums from the FILEs and check them")
	fs.BoolVar(&check, "check", false, "read MD5 sums from the FILEs and check them")
	fs.Usage = util.Usage(fs.Usage, usage)

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	rest := fs.Args()
	if check {
		if len(rest) == 0 {
			rest = []string{"-"}
		}
		for _, arg := range rest {
			if err := checkMd5File(ctx, ioEnv, arg); err != nil {
				return err
			}
		}
		return nil
	}

	if len(rest) == 0 {
		h, err := calculateMd5Sum(ctx, ioEnv.Stdin)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(ioEnv.Stdout, "%x -\n", h)
		return err
	}

	for _, arg := range rest {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if arg == "-" {
			h, err := calculateMd5Sum(ctx, ioEnv.Stdin)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(ioEnv.Stdout, "%x -\n", h); err != nil {
				return err
			}
			continue
		}

		f, err := os.Open(ioEnv.ResolvePath(arg))
		if err != nil {
			return err
		}
		h, err := calculateMd5Sum(ctx, f)
		_ = f.Close()
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(ioEnv.Stdout, "%x %s\n", h, arg); err != nil {
			return err
		}
	}
	return nil
}

// Main runs the md5sum command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "md5sum: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "md5sum: %v\n", err)
		}
		return 1
	}
	return 0
}
