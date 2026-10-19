// Copyright 2012-2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tail

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"syscall"
	"time"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type readAtSeeker interface {
	io.ReaderAt
	io.Seeker
}

type tailConfig struct {
	follow         bool
	numLines       int
	followDuration time.Duration
}

func getBlockSize(numLines int) int64 {
	if numLines <= 0 {
		return 81
	}
	return 81 * int64(numLines)
}

func lastNLines(buf []byte, n int) []byte {
	if n <= 0 || len(buf) == 0 {
		return nil
	}
	slice := buf
	var data []byte
	if len(slice) != 0 {
		if slice[len(slice)-1] == '\n' {
			slice = slice[:len(slice)-1]
		}
		var (
			foundLines int
			idx        int
		)
		for foundLines < n {
			idx = bytes.LastIndexByte(slice, '\n')
			if idx == -1 {
				break
			}
			foundLines++
			if len(slice) > 1 && idx > 0 && slice[idx-1] == '\n' {
				slice = slice[:idx]
			} else if idx > 0 {
				slice = slice[:idx-1]
			} else {
				idx = -1
				break
			}
		}
		if idx == -1 {
			data = buf
		} else {
			data = buf[idx+1:]
		}
	}
	return data
}

func readLastLinesBackwards(ctx context.Context, input readAtSeeker, writer io.Writer, numLines int) error {
	blkSize := getBlockSize(numLines)
	lastPos, err := input.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	readData := make([]byte, 0)
	buf := make([]byte, blkSize)
	pos := lastPos
	var foundLines int
	for pos != 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		thisChunkSize := min(pos, blkSize)
		pos -= thisChunkSize
		n, err := input.ReadAt(buf, pos)
		if err != nil && err != io.EOF {
			return err
		}
		n = min(n, int(thisChunkSize))
		readData = slices.Concat(buf[:n], readData)
		foundLines += bytes.Count(buf[:n], []byte{'\n'})
		if foundLines >= numLines {
			break
		}
	}
	data := lastNLines(readData, numLines)
	if _, err = writer.Write(data); err != nil {
		return err
	}
	_, err = input.Seek(lastPos, io.SeekStart)
	return err
}

func readLastLinesFromBeginning(ctx context.Context, input io.Reader, writer io.Writer, numLines int) error {
	blkSize := getBlockSize(numLines)
	buf := make([]byte, blkSize)
	var slice []byte
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := io.ReadFull(input, buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			if err != io.ErrUnexpectedEOF {
				return err
			}
		}
		slice = append(slice, buf[:n]...)
		slice = lastNLines(slice, numLines)
	}
	if _, err := writer.Write(slice); err != nil {
		return err
	}
	return nil
}

func isTruncated(file *os.File) (bool, error) {
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return false, err
	}
	return currentPos > fileInfo.Size(), nil
}

func tailReader(ctx context.Context, r io.Reader, writer io.Writer, config tailConfig) error {
	if r == nil {
		return nil
	}
	if inFile, ok := r.(*os.File); ok {
		retryFromBeginning := false
		err := readLastLinesBackwards(ctx, inFile, writer, config.numLines)
		if err != nil {
			if pathErr, ok := err.(*os.PathError); ok && (pathErr.Err == syscall.ESPIPE || pathErr.Err == syscall.EINVAL) {
				retryFromBeginning = true
			} else {
				return err
			}
		}
		if retryFromBeginning {
			if err = readLastLinesFromBeginning(ctx, inFile, writer, config.numLines); err != nil {
				return err
			}
		}
		if config.follow {
			blkSize := getBlockSize(1)
			buf := make([]byte, blkSize)
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				n, err := inFile.Read(buf)
				if err == io.EOF {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(config.followDuration):
					}
					truncated, errTruncated := isTruncated(inFile)
					if errTruncated != nil {
						break
					}
					if truncated {
						if _, errSeekStart := inFile.Seek(0, io.SeekStart); errSeekStart != nil {
							break
						}
					}
					continue
				}
				if err == nil {
					if _, err := writer.Write(buf[:n]); err != nil {
						return err
					}
					continue
				}
				break
			}
		}
		return nil
	}

	return readLastLinesFromBeginning(ctx, r, writer, config.numLines)
}

func runTail(ctx context.Context, ioEnv core.IO, follow bool, numLines int, followDuration time.Duration, args []string) error {
	if numLines < 0 {
		numLines = -numLines
	}
	config := tailConfig{follow: follow, numLines: numLines, followDuration: followDuration}

	switch len(args) {
	case 0:
		return tailReader(ctx, ioEnv.Stdin, ioEnv.Stdout, config)
	case 1:
	default:
		if follow {
			return fmt.Errorf("tail: can only read one file at a time if follow true")
		}
	}

	for i, file := range args {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if len(args) > 1 {
			fmt.Fprintf(ioEnv.Stdout, "==> %s <==\n", file)
		}
		if file == "-" {
			if err := tailReader(ctx, ioEnv.Stdin, ioEnv.Stdout, config); err != nil {
				return err
			}
		} else {
			resolved := ioEnv.ResolvePath(file)
			inFile, err := os.Open(resolved)
			if err != nil {
				return err
			}
			err = tailReader(ctx, inFile, ioEnv.Stdout, config)
			_ = inFile.Close()
			if err != nil {
				return err
			}
		}
		if len(args) > 1 && i < len(args)-1 {
			fmt.Fprintln(ioEnv.Stdout)
		}
	}
	return nil
}

// run provides backward compatibility for unit tests.
func run(reader *os.File, writer io.Writer, follow bool, numLines int, followDuration time.Duration, args []string) error {
	ioEnv := core.IO{
		Stdout: writer,
	}
	if reader != nil {
		ioEnv.Stdin = reader
	}
	return runTail(context.Background(), ioEnv, follow, numLines, followDuration, args)
}

// Main runs the tail command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "tail: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var follow bool
	var numLines int
	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&follow, "f", false, "follow the end of the file")
	fs.IntVar(&numLines, "n", 10, "specify the number of lines to show")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := runTail(ctx, ioEnv, follow, numLines, 50*time.Millisecond, fs.Args()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "tail: %v\n", err)
		return 1
	}
	return 0
}
