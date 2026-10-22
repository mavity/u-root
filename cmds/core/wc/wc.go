// Copyright 2013-2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package wc counts lines, words, runes, syntactically-invalid UTF codes.
package wc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type params struct {
	lines       bool
	words       bool
	runes       bool
	broken      bool
	chars       bool
	longestLine bool
}

type cnt struct {
	lines, words, runes, badRunes, chars, longestLine int64
}

type cmd struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	args   []string
	params
}

func command(stdin io.Reader, stdout io.Writer, stderr io.Writer, p params, args []string) *cmd {
	return &cmd{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		params: p,
		args:   args,
	}
}

func (c *cmd) run() error {
	ioEnv := core.IO{
		Stdin:  c.stdin,
		Stdout: c.stdout,
		Stderr: c.stderr,
	}
	return runWc(context.Background(), ioEnv, c.params, c.args)
}

func invalidCount(p []byte) (n int64) {
	i := 0
	for i < len(p) {
		if p[i] < utf8.RuneSelf {
			i++
		} else {
			_, size := utf8.DecodeRune(p[i:])
			if size == 1 {
				n++
			}
			i += size
		}
	}
	return
}

func countReader(ctx context.Context, in io.Reader, stderr io.Writer, fname string) (cnt, error) {
	b := bufio.NewReaderSize(in, 8192)
	counted := false
	count := cnt{}
	for !counted {
		select {
		case <-ctx.Done():
			return cnt{}, ctx.Err()
		default:
		}
		line, err := b.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				counted = true
			} else {
				if stderr != nil {
					fmt.Fprintf(stderr, "wc: %s: %v\n", fname, err)
				}
				return cnt{}, err
			}
		}
		if !counted {
			count.lines++
		}
		count.words += int64(len(bytes.Fields(line)))
		lineRunes := utf8.RuneCount(line)
		if lineRunes > 0 && line[len(line)-1] == '\n' {
			lineRunes--
			if len(line) > 1 && line[len(line)-2] == '\r' {
				lineRunes--
			}
		}
		count.runes += int64(utf8.RuneCount(line))
		if int64(lineRunes) > count.longestLine {
			count.longestLine = int64(lineRunes)
		}
		count.chars += int64(len(line))
		count.badRunes += invalidCount(line)
	}
	return count, nil
}

func report(w io.Writer, p params, count cnt, fname string) error {
	fields := []string{}
	if p.lines {
		fields = append(fields, fmt.Sprintf("%d", count.lines))
	}
	if p.words {
		fields = append(fields, fmt.Sprintf("%d", count.words))
	}
	if p.runes {
		fields = append(fields, fmt.Sprintf("%d", count.runes))
	}
	if p.broken {
		fields = append(fields, fmt.Sprintf("%d", count.badRunes))
	}
	if p.chars {
		fields = append(fields, fmt.Sprintf("%d", count.chars))
	}
	if p.longestLine {
		fields = append(fields, fmt.Sprintf("%d", count.longestLine))
	}
	if fname != "" {
		fields = append(fields, fname)
	}

	_, err := fmt.Fprintln(w, strings.Join(fields, " "))
	return err
}

func runWc(ctx context.Context, ioEnv core.IO, p params, args []string) error {
	var totals cnt
	if !p.lines && !p.words && !p.runes && !p.broken && !p.chars && !p.longestLine {
		p.lines, p.words, p.chars = true, true, true
	}

	if len(args) == 0 {
		res, err := countReader(ctx, ioEnv.Stdin, ioEnv.Stderr, "")
		if err != nil {
			return err
		}
		return report(ioEnv.Stdout, p, res, "")
	}

	var errs error

	for _, v := range args {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if v == "-" {
			res, err := countReader(ctx, ioEnv.Stdin, ioEnv.Stderr, "-")
			if err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			totals.lines += res.lines
			totals.words += res.words
			totals.runes += res.runes
			totals.badRunes += res.badRunes
			totals.chars += res.chars
			if res.longestLine > totals.longestLine {
				totals.longestLine = res.longestLine
			}
			if err := report(ioEnv.Stdout, p, res, "-"); err != nil {
				return err
			}
		} else {
			resolved := ioEnv.ResolvePath(v)
			f, err := os.Open(resolved)
			if err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			res, err := countReader(ctx, f, ioEnv.Stderr, v)
			_ = f.Close()
			if err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			totals.lines += res.lines
			totals.words += res.words
			totals.runes += res.runes
			totals.badRunes += res.badRunes
			totals.chars += res.chars
			if res.longestLine > totals.longestLine {
				totals.longestLine = res.longestLine
			}
			if err := report(ioEnv.Stdout, p, res, v); err != nil {
				return err
			}
		}
	}
	if len(args) > 1 {
		if err := report(ioEnv.Stdout, p, totals, "total"); err != nil {
			return err
		}
	}
	return errs
}

// Main runs the wc command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "wc: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var p params
	fs := flag.NewFlagSet("wc", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&p.lines, "l", false, "count lines")
	fs.BoolVar(&p.words, "w", false, "count words")
	fs.BoolVar(&p.runes, "r", false, "count runes")
	fs.BoolVar(&p.runes, "m", false, "count characters")
	fs.BoolVar(&p.broken, "b", false, "count broken")
	fs.BoolVar(&p.chars, "c", false, "count bytes (include partial UTF)")
	fs.BoolVar(&p.longestLine, "L", false, "print the length of the longest line")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runWc(ctx, ioEnv, p, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "wc: %v\n", err)
		return 1
	}
	return 0
}
