// Copyright 2013-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cpiocmd operates on cpio files using the pkg/cpio package.
package cpiocmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/cpio"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errInvalidArgs = errors.New("usage: cpio [-H format] [-v] i|o|t")

func readerAtFromStdin(r io.Reader) (io.ReaderAt, error) {
	if ra, ok := r.(io.ReaderAt); ok {
		if f, isFile := r.(*os.File); !isFile {
			return ra, nil
		} else if _, err := f.Seek(0, io.SeekCurrent); err == nil {
			return ra, nil
		}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var verbose bool
	var format string
	fs := flag.NewFlagSet("cpio", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&verbose, "v", false, "Debug prints")
	fs.StringVar(&format, "H", "newc", "format")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) < 1 {
		return errInvalidArgs
	}
	op := strings.TrimPrefix(rest[0], "-")

	archiver, err := cpio.Format(format)
	if err != nil {
		return fmt.Errorf("format %q not supported: %w", format, err)
	}

	rootDir := ioEnv.Dir
	if rootDir == "" {
		rootDir = "."
	}

	switch op {
	case "i":
		ra, err := readerAtFromStdin(ioEnv.Stdin)
		if err != nil {
			return err
		}
		rr := archiver.Reader(ra)
		inums := make(map[uint64]string)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			rec, err := rr.ReadRecord()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("error reading records: %w", err)
			}

			cleanName := filepath.ToSlash(rec.Name)
			if vol := filepath.VolumeName(cleanName); vol != "" {
				cleanName = cleanName[len(vol):]
			}
			cleanName = strings.TrimPrefix(cleanName, "/")
			if cleanName == "" {
				cleanName = "."
			}
			rec.Name = cleanName

			targetPath := filepath.Join(rootDir, cleanName)
			if rec.Ino != 0 {
				switch rec.Mode & cpio.S_IFMT {
				case cpio.S_IFDIR:
				default:
					if rec.FileSize == 0 {
						if prev, ok := inums[rec.Ino]; ok {
							if err := os.Link(prev, targetPath); err != nil {
								return err
							}
							continue
						}
					}
				}
				inums[rec.Ino] = targetPath
			}
			if err := cpio.CreateFileInRoot(rec, rootDir, false); err != nil && verbose {
				fmt.Fprintf(ioEnv.Stderr, "Creating %q failed: %v\n", rec.Name, err)
			}
		}

	case "o":
		rw := archiver.Writer(ioEnv.Stdout)
		cr := cpio.NewRecorder()
		scanner := bufio.NewScanner(ioEnv.Stdin)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			name := scanner.Text()
			resolved := ioEnv.ResolvePath(name)
			rec, err := cr.GetRecord(resolved)
			if err != nil {
				return fmt.Errorf("getting record of %q failed: %w", name, err)
			}
			rec.Name = filepath.ToSlash(name)
			if err := rw.WriteRecord(rec); err != nil {
				return fmt.Errorf("writing record %q failed: %w", name, err)
			}
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading stdin: %w", err)
		}
		if err := cpio.WriteTrailer(rw); err != nil {
			return fmt.Errorf("error writing trailer record: %w", err)
		}

	case "t":
		ra, err := readerAtFromStdin(ioEnv.Stdin)
		if err != nil {
			return err
		}
		rr := archiver.Reader(ra)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			rec, err := rr.ReadRecord()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("error reading records: %w", err)
			}
			fmt.Fprintln(ioEnv.Stdout, rec)
		}

	default:
		return errInvalidArgs
	}

	return nil
}

// Main runs the cpio command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "cpio: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "cpio: %v\n", err)
		}
		return 1
	}
	return 0
}
