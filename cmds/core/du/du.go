// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package du

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errUsage = errors.New("usage: du [-k] [-H | -L] [-a | -s] [file ...]")

type cmd struct {
	stdout            io.Writer
	stderr            io.Writer
	files             []string
	reportFiles       bool
	kbUnit            bool
	human             bool
	totalSum          bool
	followCMDSymLinks bool
	followSymlinks    bool
}

func fileBlocks512(info fs.FileInfo) int64 {
	sz := info.Size()
	if sz <= 0 {
		return 0
	}
	return (sz + 511) / 512
}

func (c *cmd) evaluateSymlink(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return "", err
	}

	symPath := path
	if fi.Mode()&os.ModeSymlink != 0 {
		symPath, err = os.Readlink(path)
		if err != nil {
			return "", err
		}

		if !filepath.IsAbs(symPath) {
			dir := filepath.Dir(path)
			symPath = filepath.Join(dir, symPath)
		}
	}

	return symPath, nil
}

func (c *cmd) printBlocks(nblock int64, path string) {
	if c.human {
		kb := (nblock + 1) / 2
		fmt.Fprintf(c.stdout, "%dK\t%s\n", kb, path)
		return
	}
	if c.kbUnit {
		nblock = (nblock + 1) / 2
	}
	fmt.Fprintf(c.stdout, "%d\t%s\n", nblock, path)
}

func (c *cmd) du(ctx context.Context, resolvedPath, displayPath string) (int64, error) {
	var blocks int64

	err := filepath.Walk(resolvedPath, func(path string, info fs.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err != nil {
			return err
		}

		disp := displayPath
		if path != resolvedPath {
			if rel, rerr := filepath.Rel(resolvedPath, path); rerr == nil {
				disp = filepath.Join(displayPath, rel)
			} else {
				disp = path
			}
		}

		if c.followSymlinks && (info.Mode()&os.ModeSymlink != 0) {
			follow, err := c.evaluateSymlink(path)
			if err != nil {
				return err
			}
			symBlocks, err := c.du(ctx, follow, disp)
			if err != nil {
				return err
			}
			blocks += symBlocks
			return nil
		}

		// report sub-folders and add number of blocks to overall count
		if info.IsDir() && path != resolvedPath && !c.totalSum {
			dirBlocks, err := c.du(ctx, path, disp)
			if err != nil {
				return err
			}

			blocks += dirBlocks
			c.printBlocks(dirBlocks, disp)
			return fs.SkipDir
		}

		nb := fileBlocks512(info)
		if c.reportFiles && !info.IsDir() && !c.totalSum {
			c.printBlocks(nb, disp)
		}

		blocks += nb
		return nil
	})

	return blocks, err
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	c := cmd{
		stdout: ioEnv.Stdout,
		stderr: ioEnv.Stderr,
	}

	f := flag.NewFlagSet("du", flag.ContinueOnError)
	f.SetOutput(ioEnv.Stderr)
	f.BoolVar(&c.reportFiles, "a", false, "report the size of each file not of type directory")
	f.BoolVar(&c.kbUnit, "k", false, "write the files sizes in units of 1024 bytes, rather than the default 512-byte units")
	f.BoolVar(&c.human, "h", false, "print sizes in human readable format")
	f.BoolVar(&c.totalSum, "s", false, "report only the total sum for each of the specified files")
	f.BoolVar(&c.followCMDSymLinks, "H", false, "follow symlink form [file...]")
	f.BoolVar(&c.followSymlinks, "L", false, "follow all symlinks")

	if err := f.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}
	c.files = f.Args()

	if c.totalSum && c.reportFiles {
		return errUsage
	}

	if c.followSymlinks && c.followCMDSymLinks {
		return errUsage
	}

	if len(c.files) == 0 {
		c.files = append(c.files, ".")
	}

	for _, file := range c.files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resolved := ioEnv.ResolvePath(file)
		if c.followCMDSymLinks {
			var err error
			resolved, err = c.evaluateSymlink(resolved)
			if err != nil {
				return err
			}
		}

		blocks, err := c.du(ctx, resolved, file)
		if err != nil {
			return err
		}
		c.printBlocks(blocks, file)
	}

	return nil
}

// Main runs the du command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "du: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "du: %v\n", err)
		}
		return 1
	}
	return 0
}
