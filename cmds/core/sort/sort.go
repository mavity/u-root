// Copyright 2017-2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sort sorts lines.
package sort

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	stdsort "sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type ignoreCaseSort []string

func (a ignoreCaseSort) Len() int           { return len(a) }
func (a ignoreCaseSort) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ignoreCaseSort) Less(i, j int) bool { return strings.ToUpper(a[i]) < strings.ToUpper(a[j]) }

type ignoreBlanksSort []string

func (a ignoreBlanksSort) Len() int      { return len(a) }
func (a ignoreBlanksSort) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ignoreBlanksSort) Less(i, j int) bool {
	l := strings.TrimLeftFunc(a[i], unicode.IsSpace)
	r := strings.TrimLeftFunc(a[j], unicode.IsSpace)
	if l == r {
		return len(a[i]) > len(a[j])
	}
	return l < r
}

type ignoreBlanksCaseSort []string

func (a ignoreBlanksCaseSort) Len() int      { return len(a) }
func (a ignoreBlanksCaseSort) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ignoreBlanksCaseSort) Less(i, j int) bool {
	l := strings.ToUpper(strings.TrimLeftFunc(a[i], unicode.IsSpace))
	r := strings.ToUpper(strings.TrimLeftFunc(a[j], unicode.IsSpace))
	if l == r {
		if len(a[i]) != len(a[j]) {
			return len(a[i]) > len(a[j])
		}
		return a[i] < a[j]
	}
	return l < r
}

type numericSort []string

func (a numericSort) Len() int      { return len(a) }
func (a numericSort) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a numericSort) Less(i, j int) bool {
	l := strings.ToUpper(strings.TrimLeftFunc(a[i], unicode.IsSpace))
	r := strings.ToUpper(strings.TrimLeftFunc(a[j], unicode.IsSpace))

	ln, _ := strconv.ParseFloat(l, 64)
	rn, _ := strconv.ParseFloat(r, 64)

	return ln < rn
}

// ErrNotOrdered is returned when sort -C finds input is not ordered.
var ErrNotOrdered = errors.New("not ordered")
var errNotOrdered = ErrNotOrdered

type params struct {
	outputFile   string
	reverse      bool
	ordered      bool
	unique       bool
	ignoreCase   bool
	ignoreBlanks bool
	numeric      bool
}

type cmd struct {
	stdin  io.ReadCloser
	stdout io.Writer
	stderr io.Writer
	params params
	args   []string
}

func command(stdin io.ReadCloser, stdout, stderr io.Writer, p params, args []string) *cmd {
	return &cmd{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		params: p,
		args:   args,
	}
}

func sortInterface(p params, lines []string) stdsort.Interface {
	var si stdsort.Interface
	switch {
	case p.numeric:
		si = numericSort(lines)
	case p.ignoreBlanks && p.ignoreCase:
		si = ignoreBlanksCaseSort(lines)
	case p.ignoreBlanks:
		si = ignoreBlanksSort(lines)
	case p.ignoreCase:
		si = ignoreCaseSort(lines)
	default:
		si = stdsort.StringSlice(lines)
	}
	return si
}

func sameSortKey(p params, left, right string) bool {
	if p.numeric {
		l, _ := strconv.ParseFloat(strings.ToUpper(strings.TrimLeftFunc(left, unicode.IsSpace)), 64)
		r, _ := strconv.ParseFloat(strings.ToUpper(strings.TrimLeftFunc(right, unicode.IsSpace)), 64)
		return l == r || (l != l && r != r)
	}
	if p.ignoreBlanks {
		left = strings.TrimLeftFunc(left, unicode.IsSpace)
		right = strings.TrimLeftFunc(right, unicode.IsSpace)
	}
	if p.ignoreCase {
		return strings.ToUpper(left) == strings.ToUpper(right)
	}
	return left == right
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if strings.HasSuffix(s, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func sortAlgorithm(p params, s string) string {
	lines := splitLines(s)
	if len(lines) == 0 {
		return ""
	}
	si := sortInterface(p, lines)

	if p.reverse {
		stdsort.Stable(stdsort.Reverse(si))
	} else {
		stdsort.Stable(si)
	}

	if p.unique && len(lines) > 1 {
		write := 1
		for i := 1; i < len(lines); i++ {
			if sameSortKey(p, lines[write-1], lines[i]) {
				continue
			}
			lines[write] = lines[i]
			write++
		}
		lines = lines[:write]
	}

	return strings.Join(lines, "\n") + "\n"
}

func (c *cmd) run() error {
	ioEnv := core.IO{
		Stdin:  c.stdin,
		Stdout: c.stdout,
		Stderr: c.stderr,
	}
	return runSort(context.Background(), ioEnv, c.params, c.args)
}

func (c *cmd) sortInterface(lines []string) stdsort.Interface {
	return sortInterface(c.params, lines)
}

func (c *cmd) sortAlgorithm(s string) string {
	return sortAlgorithm(c.params, s)
}

func runSort(ctx context.Context, ioEnv core.IO, p params, args []string) error {
	from := []io.ReadCloser{}
	for _, v := range args {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resolved := ioEnv.ResolvePath(v)
		f, err := os.Open(resolved)
		if err != nil {
			return err
		}
		defer f.Close()
		from = append(from, f)
	}

	if len(args) == 0 {
		if ioEnv.Stdin != nil {
			if rc, ok := ioEnv.Stdin.(io.ReadCloser); ok {
				from = append(from, rc)
			} else {
				from = append(from, io.NopCloser(ioEnv.Stdin))
			}
		}
	}

	fileContents := []string{}
	for _, f := range from {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		bytes, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		s := string(bytes)
		fileContents = append(fileContents, s)
		if len(s) > 0 && s[len(s)-1] != '\n' {
			fileContents = append(fileContents, "\n")
		}
	}

	s := strings.Join(fileContents, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")

	if p.ordered {
		lines := splitLines(s)
		si := sortInterface(p, lines)
		if p.reverse {
			si = stdsort.Reverse(si)
		}
		if !stdsort.IsSorted(si) {
			return ErrNotOrdered
		}

		if p.unique && len(lines) > 1 {
			for i := 1; i < len(lines); i++ {
				if sameSortKey(p, lines[i], lines[i-1]) {
					return ErrNotOrdered
				}
			}
		}

		return nil
	}

	outStr := sortAlgorithm(p, s)
	to := ioEnv.Stdout
	if p.outputFile != "" {
		outPath := ioEnv.ResolvePath(p.outputFile)
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer f.Close()
		to = f
	}

	_, err := to.Write([]byte(outStr))
	return err
}

// Main runs the sort command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "sort: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var p params
	fs := flag.NewFlagSet("sort", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&p.reverse, "r", false, "Reverse the result of comparisons.")
	fs.BoolVar(&p.ordered, "C", false, "Check that the single input file is ordered. No warnings.")
	fs.BoolVar(&p.unique, "u", false, "Unique keys. Suppress all lines that have a key that is equal to an already processed one.")
	fs.BoolVar(&p.ignoreCase, "f", false, "Fold lower case to upper case character.")
	fs.BoolVar(&p.ignoreBlanks, "b", false, "Ignore leading blank characters when comparing lines.")
	fs.BoolVar(&p.numeric, "n", false, "Compare according to string numerical value.")
	fs.StringVar(&p.outputFile, "o", "", "Specify the name of an output file to be used instead of the standard output.")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runSort(ctx, ioEnv, p, fs.Args()); err != nil {
		if errors.Is(err, ErrNotOrdered) {
			return 1
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "sort: %v\n", err)
		return 1
	}
	return 0
}
