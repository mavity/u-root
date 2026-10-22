// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package seq prints a sequence of numbers.
package seq

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

const cmd = "seq [-f format] [-w] [-s separator] [start [step [end]]]"

var (
	ErrUsage       = errors.New(cmd)
	ErrNegativeDec = errors.New("needs negative decrement")
	ErrPositiveInc = errors.New("needs positive increment")
	ErrZeroDec     = errors.New("zero decrement")

	errUsage       = ErrUsage
	errNegativeDec = ErrNegativeDec
	errPositiveInc = ErrPositiveInc
	errZeroDec     = ErrZeroDec
)

type s struct {
	first  float64
	incr   float64
	last   float64
	format string
}

func maxDecimalPlaces(args []string) int {
	m := 0
	for _, a := range args {
		if i := strings.Index(a, "."); i >= 0 {
			m = max(m, len(a)-i-1)
		}
	}
	return m
}

func parse(format string, args []string) (s, error) {
	s1 := s{format: format, first: 1, incr: 1}
	argc := len(args)
	switch argc {
	case 3: // first, incr, last
		first, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			return s1, err
		}
		s1.first = first

		incr, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			return s1, err
		}
		s1.incr = incr

		if format == "%v" {
			d := maxDecimalPlaces(args)
			if d > 0 {
				s1.format = fmt.Sprintf("%%.%df", d)
			}
		}

		last, err := strconv.ParseFloat(args[2], 64)
		if err != nil {
			return s1, err
		}
		s1.last = last

		// handle wrong inc errors
		if s1.incr == 0 {
			return s1, errZeroDec
		}
		if s1.first > s1.last && s1.incr >= 0 {
			return s1, errNegativeDec
		}
		if s1.first < s1.last && s1.incr <= 0 {
			return s1, errPositiveInc
		}
	case 2: // first, last
		first, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			return s1, err
		}
		s1.first = first
		last, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			return s1, err
		}
		s1.last = last
	case 1: // last
		last, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			return s1, err
		}
		s1.last = last
	default:
		return s1, errUsage
	}

	return s1, nil
}

func runSeq(ctx context.Context, w io.Writer, format string, separator string, widthEqual bool, args []string) error {
	s1, err := parse(format, args)
	if err != nil {
		return err
	}

	// set default inc if needed
	if s1.first > s1.last && s1.incr == 1 {
		s1.incr = -1
	}

	format = s1.format
	var width int
	if widthEqual {
		format = strings.Replace(format, "%", "%0*", 1)
		width = max(
			len(fmt.Sprintf(format, 0, s1.first)),
			len(fmt.Sprintf(format, 0, s1.last)),
		)
	}

	inRange := func(value float64) bool {
		if s1.incr > 0 {
			return value <= s1.last
		}
		return value >= s1.last
	}

	first := true
	for value := s1.first; inRange(value); {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if !first {
			if _, err := fmt.Fprint(w, separator); err != nil {
				return err
			}
		}
		var err error
		if widthEqual {
			_, err = fmt.Fprintf(w, format, width, value)
		} else {
			_, err = fmt.Fprintf(w, format, value)
		}
		if err != nil {
			return err
		}
		first = false
		next := value + s1.incr
		if next == value && value != s1.last {
			return errors.New("sequence increment is too small to advance")
		}
		value = next
	}
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return err
	}
	return nil
}

// seq provides backward compatibility for unit tests.
func seq(w io.Writer, format string, separator string, widthEqual bool, args []string) error {
	return runSeq(context.Background(), w, format, separator, widthEqual, args)
}

// Main runs the seq command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "seq: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var (
		format     string
		separator  string
		widthEqual bool
	)
	fs := flag.NewFlagSet("seq", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.StringVar(&format, "f", "%v", "use printf style floating-point FORMAT")
	fs.StringVar(&separator, "s", "\n", "use STRING to separate numbers")
	fs.BoolVar(&widthEqual, "w", false, "equalize width by padding with leading zeroes")

	flagArgs := args
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			break
		}
		if arg == "-f" || arg == "-s" {
			i++
			continue
		}
		if _, err := strconv.ParseFloat(arg, 64); err == nil {
			flagArgs = append(append(append([]string(nil), args[:i]...), "--"), args[i:]...)
			break
		}
	}
	if err := fs.Parse(unixflag.ArgsToGoArgs(flagArgs)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runSeq(ctx, ioEnv.Stdout, format, separator, widthEqual, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "seq: %v\n", err)
		return 1
	}
	return 0
}
