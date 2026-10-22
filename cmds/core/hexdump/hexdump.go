// Copyright 2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hexdump prints file content in hexadecimal.
package hexdump

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type hexdumpFormatter struct {
	writer       io.Writer
	offset       uint64
	canonical    bool
	verbose      bool
	row          []byte
	previousLine string
	havePrevious bool
	suppressed   bool
	closed       bool
}

func (w *hexdumpFormatter) writeString(s string) error {
	n, err := io.WriteString(w.writer, s)
	if err != nil {
		return err
	}
	if n != len(s) {
		return io.ErrShortWrite
	}
	return nil
}

func formatRow(row []byte, canonical bool) string {
	var line strings.Builder
	if canonical {
		for i := 0; i < 16; i++ {
			if i == 8 {
				line.WriteByte(' ')
			}
			if i < len(row) {
				fmt.Fprintf(&line, "%02x ", row[i])
			} else {
				line.WriteString("   ")
			}
		}
		line.WriteString(" |")
		for i := 0; i < len(row); i++ {
			if row[i] >= 0x20 && row[i] <= 0x7e {
				line.WriteByte(row[i])
			} else {
				line.WriteByte('.')
			}
		}
		line.WriteString("|\n")
		return line.String()
	}

	for i := 0; i < 16; i += 2 {
		if i >= len(row) {
			line.WriteString("     ")
			continue
		}
		var pair [2]byte
		copy(pair[:], row[i:min(i+2, len(row))])
		fmt.Fprintf(&line, "%04x ", binary.NativeEndian.Uint16(pair[:]))
	}
	line.WriteByte('\n')
	return line.String()
}

func (w *hexdumpFormatter) writeRow(row []byte) error {
	line := formatRow(row, w.canonical)
	if !w.verbose && w.havePrevious && line == w.previousLine {
		w.suppressed = true
		w.offset += uint64(len(row))
		return nil
	}

	if w.suppressed {
		if err := w.writeString("*\n"); err != nil {
			return err
		}
		w.suppressed = false
	}

	prefix := fmt.Sprintf("%07x ", w.offset)
	if w.canonical {
		prefix = fmt.Sprintf("%08x  ", w.offset)
	}
	if err := w.writeString(prefix + line); err != nil {
		return err
	}
	w.previousLine = line
	w.havePrevious = true
	w.offset += uint64(len(row))
	return nil
}

func (w *hexdumpFormatter) Write(p []byte) error {
	for len(p) > 0 {
		n := min(16-len(w.row), len(p))
		w.row = append(w.row, p[:n]...)
		p = p[n:]
		if len(w.row) == 16 {
			if err := w.writeRow(w.row); err != nil {
				return err
			}
			w.row = w.row[:0]
		}
	}
	return nil
}

func (w *hexdumpFormatter) Close(writeFinalOffset bool) error {
	if w.closed {
		return nil
	}
	w.closed = true
	if len(w.row) > 0 {
		if err := w.writeRow(w.row); err != nil {
			return err
		}
		w.row = w.row[:0]
	}
	if w.suppressed {
		if err := w.writeString("*\n"); err != nil {
			return err
		}
		w.suppressed = false
	}
	if writeFinalOffset {
		width := 7
		if w.canonical {
			width = 8
		}
		return w.writeString(fmt.Sprintf("%0*x\n", width, w.offset))
	}
	return nil
}

func runHexdump(ctx context.Context, ioEnv core.IO, filenames []string, maxBytes, skip int64, canonical, verbose bool) (retErr error) {
	var readers []io.Reader
	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			if err := c.Close(); retErr == nil && err != nil {
				retErr = err
			}
		}
	}()

	if len(filenames) == 0 {
		readers = []io.Reader{ioEnv.Stdin}
	} else {
		readers = make([]io.Reader, 0, len(filenames))
		for _, filename := range filenames {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if filename == "-" {
				readers = append(readers, ioEnv.Stdin)
			} else {
				resolved := ioEnv.ResolvePath(filename)
				f, err := os.Open(resolved)
				if err != nil {
					return err
				}
				closers = append(closers, f)
				readers = append(readers, f)
			}
		}
	}

	r := io.MultiReader(readers...)
	if skip > 0 {
		_, err := io.CopyN(io.Discard, r, skip)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
	if maxBytes >= 0 {
		r = io.LimitReader(r, maxBytes)
	}

	w := &hexdumpFormatter{
		writer:    ioEnv.Stdout,
		offset:    uint64(skip),
		canonical: canonical,
		verbose:   verbose,
	}
	writeFinalOffset := false
	defer func() {
		if err := w.Close(writeFinalOffset); retErr == nil && err != nil {
			retErr = err
		}
	}()

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		nr, er := r.Read(buf)
		if nr > 0 {
			if err := w.Write(buf[:nr]); err != nil {
				return err
			}
		}
		if er != nil {
			if er == io.EOF {
				writeFinalOffset = true
				break
			}
			return er
		}
	}

	return nil
}

// hexdump provides backward compatibility for unit tests.
func hexdump(filenames []string, reader io.Reader, writer io.Writer) error {
	ioEnv := core.IO{
		Stdin:  reader,
		Stdout: writer,
	}
	return runHexdump(context.Background(), ioEnv, filenames, -1, 0, true, false)
}

// Main runs the hexdump command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "hexdump: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var (
		canonical bool
		verbose   bool
		maxBytes  int64
		skip      int64
	)
	maxBytes = -1
	fs := flag.NewFlagSet("hexdump", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&canonical, "C", false, "canonical hex+ASCII display")
	fs.BoolVar(&verbose, "v", false, "do not suppress repeated output lines")
	fs.Int64Var(&maxBytes, "n", -1, "interpret only N bytes of input")
	fs.Int64Var(&skip, "s", 0, "skip N bytes from the beginning")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if maxBytes < -1 || skip < 0 {
		fmt.Fprintln(ioEnv.Stderr, "hexdump: byte counts must not be negative")
		return 2
	}
	_ = canonical
	_ = verbose
	if err := runHexdump(ctx, ioEnv, fs.Args(), maxBytes, skip, canonical, verbose); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "hexdump: %v\n", err)
		return 1
	}
	return 0
}
