// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// dd converts and copies a file.
//
// Synopsis:
//
//	dd [OPTIONS...] [-inName FILE] [-outName FILE]
//
// Description:
//
//	dd is modeled after dd(1).
//
// Options:
//
//	-ibs n:   input block size (default=1)
//	-obs n:   output block size (default=1)
//	-bs n:    input and output block size (default=0)
//	-skip n:  skip n ibs-sized input blocks before reading (default=0)
//	-seek n:  seek n obs-sized output blocks before writing (default=0)
//	-conv s:  comma separated list of conversions (none|notrunc)
//	-count n: copy only n ibs-sized input blocks
//	-if:      defaults to stdin
//	-of:      defaults to stdout
//	-oflag:   comma separated list of out flags (none|sync|dsync)
//	-status:  print transfer stats to stderr, can be one of:
//	    none:     do not display
//	    xfer:     print on completion (default)
//	    progress: print throughout transfer (GNU)
//
// Notes:
//
//	Because UTF-8 clashes with block-oriented copying, `conv=lcase` and
//	`conv=ucase` will not be supported. Additionally, research showed these
//	arguments are rarely useful. Use tr instead.
package dd

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/rck/unit"
	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/progress"
)

type bitClearAndSet struct {
	clear int
	set   int
}

// N.B. The flags in os derive from syscall.
// They are, hence, mutually exclusive, on target
// kernels.
var convMap = map[string]bitClearAndSet{
	"notrunc": {clear: os.O_TRUNC},
}

var flagMap = map[string]bitClearAndSet{
	"sync": {set: os.O_SYNC},
}

var allowedFlags = os.O_TRUNC | os.O_SYNC

func dd(r io.Reader, w io.Writer, inBufSize, outBufSize int64, bytesWritten *int64) error {
	return ddWithCtx(context.Background(), r, w, inBufSize, outBufSize, bytesWritten)
}

func ddWithCtx(ctx context.Context, r io.Reader, w io.Writer, inBufSize, outBufSize int64, bytesWritten *int64) error {
	if inBufSize == 0 {
		return fmt.Errorf("inBufSize is not allowed to be zero")
	}
	// There is an optimization in the Go runtime for zero-copy,
	// which we can use when inBufSize == outBufSize.
	for inBufSize == outBufSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		amt, err := io.CopyN(w, r, inBufSize)
		*bytesWritten += int64(amt)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
	dat := &bytes.Buffer{}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		for int64(dat.Len()) > outBufSize {
			if _, err := io.CopyN(w, dat, outBufSize); err != nil {
				return err
			}
		}
		if _, err := io.CopyN(dat, r, inBufSize); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	for dat.Len() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := io.CopyN(w, dat, outBufSize); err != nil && err != io.EOF {
			return err
		}
	}
	return nil
}

// sectionReader implements a SectionReader on an underlying implementation of
// io.Reader (as opposed to io.SectionReader which uses io.ReaderAt).
type sectionReader struct {
	base   int64
	offset int64
	limit  int64
	io.Reader
}

// newStreamSectionReader uses an io.Reader to implement an io.Reader that
// seeks to offset and reads at most n bytes.
//
// This is useful if you want to use a NewSectionReader with stdin or other
// types of pipes (things that can't be seek'd or pread from).
func newStreamSectionReader(r io.Reader, offset int64, n int64) io.Reader {
	limit := offset + n
	if limit < 0 {
		limit = math.MaxInt64
	}
	return &sectionReader{offset, 0, limit, r}
}

// Read implements io.Reader.
func (s *sectionReader) Read(p []byte) (int, error) {
	if s.offset == 0 && s.base != 0 {
		if n, err := io.CopyN(io.Discard, s.Reader, s.base); err != nil {
			return 0, err
		} else if n != s.base {
			// Can't happen.
			return 0, fmt.Errorf("error skipping input bytes, short write")
		}
		s.offset = s.base
	}

	if s.offset >= s.limit {
		return 0, io.EOF
	}

	if mx := s.limit - s.offset; int64(len(p)) > mx {
		p = p[0:mx]
	}

	n, err := s.Reader.Read(p)
	s.offset += int64(n)

	// Convert to io.EOF explicitly.
	if n == 0 && err == nil {
		return 0, io.EOF
	}
	return n, err
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func (zeroReader) ReadAt(p []byte, off int64) (int, error) {
	clear(p)
	return len(p), nil
}

type closerSectionReader struct {
	*io.SectionReader
	io.Closer
}

func (c closerSectionReader) Close() error {
	if c.Closer != nil {
		return c.Closer.Close()
	}
	return nil
}

// openInput opens the input stream or file and seeks to the right position.
func openInput(ioEnv core.IO, name string, inputBytes int64, skip int64, count int64) (io.Reader, io.Closer, error) {
	maxRead := int64(math.MaxInt64)
	if count != math.MaxInt64 {
		maxRead = count * inputBytes
	}

	if name == "" {
		stdin := ioEnv.Stdin
		if stdin == nil {
			stdin = strings.NewReader("")
		}
		return newStreamSectionReader(stdin, inputBytes*skip, maxRead), nil, nil
	}

	if name == "/dev/zero" {
		return io.NewSectionReader(zeroReader{}, inputBytes*skip, maxRead), nil, nil
	}
	if name == "/dev/null" {
		return strings.NewReader(""), nil, nil
	}

	resolved := ioEnv.ResolvePath(name)
	in, err := os.Open(resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("error opening input file %q: %w", name, err)
	}
	return io.NewSectionReader(in, inputBytes*skip, maxRead), in, nil
}

// openOutput opens the output stream or file and seeks to the right position.
func openOutput(ioEnv core.IO, name string, outputBytes int64, seek int64, flags int) (io.Writer, io.Closer, error) {
	if name == "" {
		out := ioEnv.Stdout
		if out == nil {
			out = io.Discard
		}
		if seek*outputBytes != 0 {
			if ws, ok := out.(io.WriteSeeker); ok {
				if _, err := ws.Seek(seek*outputBytes, io.SeekCurrent); err != nil {
					return nil, nil, fmt.Errorf("error seeking output: %w", err)
				}
			} else {
				return nil, nil, fmt.Errorf("cannot seek on non-seekable output stdout")
			}
		}
		return out, nil, nil
	}

	if name == "/dev/null" {
		return io.Discard, nil, nil
	}

	resolved := ioEnv.ResolvePath(name)
	perm := os.O_CREATE | os.O_WRONLY | (flags & allowedFlags)
	f, err := os.OpenFile(resolved, perm, 0o666)
	if err != nil {
		return nil, nil, fmt.Errorf("error opening output file %q: %w", name, err)
	}
	if seek*outputBytes != 0 {
		if _, err := f.Seek(seek*outputBytes, io.SeekCurrent); err != nil {
			_ = f.Close()
			return nil, nil, fmt.Errorf("error seeking output file: %w", err)
		}
	}
	return f, f, nil
}

// inFile provides backward compatibility for existing callers and tests.
func inFile(stdin io.Reader, name string, inputBytes int64, skip int64, count int64) (io.Reader, error) {
	r, closer, err := openInput(core.IO{Stdin: stdin}, name, inputBytes, skip, count)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		if sr, ok := r.(*io.SectionReader); ok {
			return closerSectionReader{SectionReader: sr, Closer: closer}, nil
		}
	}
	return r, nil
}

// outFile provides backward compatibility for existing callers and tests.
func outFile(stdout io.WriteSeeker, name string, outputBytes int64, seek int64, flags int) (io.Writer, error) {
	w, _, err := openOutput(core.IO{Stdout: stdout}, name, outputBytes, seek, flags)
	return w, err
}

func usageErr() error {
	return errors.New(`Usage: dd [if=file] [of=file] [conv=none|notrunc] [seek=#] [skip=#]
			     [count=#] [bs=#] [ibs=#] [obs=#] [status=none|xfer|progress] [oflag=none|sync|dsync]
		options may also be invoked Go-style as -opt value or -opt=value
		bs, if specified, overrides ibs and obs`)
}

func convertArgs(osArgs []string) []string {
	var args []string
	for _, v := range osArgs {
		l := strings.SplitN(v, "=", 2)
		if len(l) == 2 {
			l[0] = "-" + l[0]
		}
		args = append(args, l...)
	}
	return args
}

// Main executes the dd command with the given context and I/O environment.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "dd: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := runCmd(ctx, ioEnv, args); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "dd: %v\n", err)
		return 1
	}
	return 0
}

// run provides backward compatibility for existing tests.
func run(stdin io.Reader, stdout io.WriteSeeker, stderr io.Writer, name string, args []string) error {
	ioEnv := core.IO{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	return runCmd(context.Background(), ioEnv, args)
}

func runCmd(ctx context.Context, ioEnv core.IO, args []string) error {
	f := flag.NewFlagSet("dd", flag.ContinueOnError)
	f.SetOutput(ioEnv.Stderr)

	var (
		skip    = f.Int64("skip", 0, "skip N ibs-sized blocks before reading")
		seek    = f.Int64("seek", 0, "seek N obs-sized blocks before writing")
		conv    = f.String("conv", "none", "comma separated list of conversions (none|notrunc)")
		count   = f.Int64("count", math.MaxInt64, "copy only N input blocks")
		inName  = f.String("if", "", "Input file")
		outName = f.String("of", "", "Output file")
		oFlag   = f.String("oflag", "none", "comma separated list of out flags (none|sync|dsync)")
		status  = f.String("status", "xfer", "display status of transfer (none|xfer|progress)")
	)
	ddUnits := unit.DefaultUnits
	ddUnits["c"] = 1
	ddUnits["w"] = 2
	ddUnits["b"] = 512
	delete(ddUnits, "B")

	var (
		ibs = unit.MustNewUnit(ddUnits).MustNewValue(math.MaxInt64, unit.None)
		obs = unit.MustNewUnit(ddUnits).MustNewValue(math.MaxInt64, unit.None)
		bs  = unit.MustNewUnit(ddUnits).MustNewValue(math.MaxInt64, unit.None)
	)
	f.Var(ibs, "ibs", "Default input block size")
	f.Var(obs, "obs", "Default output block size")
	f.Var(bs, "bs", "Default input and output block size")

	args = convertArgs(args)
	if err := f.Parse(args); err != nil {
		return err
	}

	if len(f.Args()) > 0 {
		return usageErr()
	}

	// Convert conv argument to bit set.
	flags := os.O_TRUNC
	if *conv != "none" {
		for c := range strings.SplitSeq(*conv, ",") {
			if v, ok := convMap[c]; ok {
				flags &= ^v.clear
				flags |= v.set
			} else {
				return fmt.Errorf("unknown argument conv=%s", c)
			}
		}
	}

	// Convert oflag argument to bit set.
	if *oFlag != "none" {
		for fl := range strings.SplitSeq(*oFlag, ",") {
			if v, ok := flagMap[fl]; ok {
				flags &= ^v.clear
				flags |= v.set
			} else {
				return fmt.Errorf("unknown argument oflag=%s", fl)
			}
		}
	}

	if *status != "none" && *status != "xfer" && *status != "progress" {
		return usageErr()
	}

	var bytesWritten int64
	p := progress.New(ioEnv.Stderr, *status, &bytesWritten)
	p.Begin()
	defer p.End()

	// bs = both 'ibs' and 'obs' (IEEE Std 1003.1 - 2013)
	if bs.IsSet {
		ibs = bs
		obs = bs
	}

	in, inCloser, err := openInput(ioEnv, *inName, ibs.Value, *skip, *count)
	if err != nil {
		return err
	}
	if inCloser != nil {
		defer inCloser.Close()
	}

	out, outCloser, err := openOutput(ioEnv, *outName, obs.Value, *seek, flags)
	if err != nil {
		return err
	}
	if outCloser != nil {
		defer outCloser.Close()
	}

	return ddWithCtx(ctx, in, out, ibs.Value, obs.Value, &bytesWritten)
}
