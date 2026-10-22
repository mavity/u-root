// Copyright 2012-2022 the u-root Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tail

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type tailConfig struct {
	follow         bool
	numLines       int64
	numBytes       int64
	byteCount      bool
	followDuration time.Duration
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func tailLines(ctx context.Context, input io.Reader, writer io.Writer, count int64) error {
	reader := bufio.NewReader(input)
	lines := make([][]byte, 0, min(int(count), 1024))
	next, stored := 0, 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadBytes('\n')
		if len(line) > 0 && count > 0 {
			if int64(len(lines)) < count {
				lines = append(lines, line)
				stored++
				next = len(lines) % int(count)
			} else {
				lines[next] = line
				next = (next + 1) % len(lines)
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
			break
		}
	}

	start := 0
	if stored == len(lines) && int64(stored) == count && len(lines) > 0 {
		start = next
	}
	for i := 0; i < stored; i++ {
		if err := writeAll(writer, lines[(start+i)%len(lines)]); err != nil {
			return err
		}
	}
	return nil
}

func tailBytes(ctx context.Context, input io.Reader, writer io.Writer, count int64) error {
	target := int(count)
	buffer := make([]byte, min(target, 32*1024))
	readBuffer := make([]byte, 32*1024)
	writeAt, stored := 0, 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := input.Read(readBuffer)
		if n > 0 {
			if len(buffer) > 0 {
				for _, b := range readBuffer[:n] {
					if stored == len(buffer) && stored < target {
						newSize := min(target, max(len(buffer)*2, 32*1024))
						grown := make([]byte, newSize)
						copy(grown, buffer)
						buffer = grown
						writeAt = stored
					}
					buffer[writeAt] = b
					writeAt = (writeAt + 1) % len(buffer)
					if stored < target {
						stored++
					}
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
			break
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	if len(buffer) == 0 {
		return nil
	}
	size := stored
	start := 0
	if stored == target {
		start = writeAt
	}
	first := min(size, len(buffer)-start)
	if err := writeAll(writer, buffer[start:start+first]); err != nil {
		return err
	}
	return writeAll(writer, buffer[:size-first])
}

func isTruncated(file *os.File) (bool, error) {
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	return currentPos > info.Size(), nil
}

func followFile(ctx context.Context, file *os.File, writer io.Writer, interval time.Duration) error {
	buffer := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := file.Read(buffer)
		if n > 0 {
			if writeErr := writeAll(writer, buffer[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err == nil {
			if n == 0 {
				return io.ErrNoProgress
			}
			continue
		}
		if !errors.Is(err, io.EOF) {
			return err
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		truncated, err := isTruncated(file)
		if err != nil {
			return err
		}
		if truncated {
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return err
			}
		}
	}
}

func tailReader(ctx context.Context, reader io.Reader, writer io.Writer, config tailConfig) error {
	if reader == nil {
		return errors.New("tail: stdin is unavailable")
	}
	var err error
	if config.byteCount {
		err = tailBytes(ctx, reader, writer, config.numBytes)
	} else {
		err = tailLines(ctx, reader, writer, config.numLines)
	}
	if err != nil {
		return err
	}
	if file, ok := reader.(*os.File); config.follow && ok {
		return followFile(ctx, file, writer, config.followDuration)
	}
	return nil
}

func runTail(ctx context.Context, ioEnv core.IO, config tailConfig, args []string) error {
	if ioEnv.Stdout == nil {
		return errors.New("tail: stdout is unavailable")
	}
	if config.follow && len(args) > 1 {
		return errors.New("tail: can only follow one file at a time")
	}

	if len(args) == 0 {
		return tailReader(ctx, ioEnv.Stdin, ioEnv.Stdout, config)
	}
	for i, name := range args {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if len(args) > 1 {
			if i > 0 {
				if _, err := fmt.Fprintln(ioEnv.Stdout); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(ioEnv.Stdout, "==> %s <==\n", name); err != nil {
				return err
			}
		}
		if name == "-" {
			if err := tailReader(ctx, ioEnv.Stdin, ioEnv.Stdout, config); err != nil {
				return fmt.Errorf("standard input: %w", err)
			}
			continue
		}

		file, err := os.Open(ioEnv.ResolvePath(name))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		err = tailReader(ctx, file, ioEnv.Stdout, config)
		closeErr := file.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if closeErr != nil {
			return fmt.Errorf("%s: %w", name, closeErr)
		}
	}
	return nil
}

// Main runs the tail command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	stderr := ioEnv.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "tail: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	fs.SetOutput(stderr)
	follow := fs.Bool("f", false, "follow the end of the file")
	numLines := fs.Int64("n", 10, "output the last COUNT lines")
	numBytes := fs.Int64("c", -1, "output the last COUNT bytes")
	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	hasLines, hasBytes := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "n":
			hasLines = true
		case "c":
			hasBytes = true
		}
	})
	if hasLines && hasBytes {
		fmt.Fprintln(stderr, "tail: cannot combine -c and -n")
		return 2
	}
	count := *numLines
	if hasBytes {
		count = *numBytes
	}
	if count < 0 {
		if count == -1<<63 {
			fmt.Fprintln(stderr, "tail: count is too large")
			return 2
		}
		count = -count
	}
	if uint64(count) > uint64(^uint(0)>>1) {
		fmt.Fprintln(stderr, "tail: count is too large")
		return 2
	}
	if *follow && len(fs.Args()) > 1 {
		fmt.Fprintln(stderr, "tail: can only follow one file at a time")
		return 1
	}

	config := tailConfig{
		follow:         *follow,
		numLines:       count,
		numBytes:       count,
		byteCount:      hasBytes,
		followDuration: 50 * time.Millisecond,
	}
	if err := runTail(ctx, ioEnv, config, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		if isBrokenPipe(err) {
			return 0
		}
		fmt.Fprintf(stderr, "tail: %v\n", err)
		return 1
	}
	return 0
}

func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "pipe is being closed")
}
