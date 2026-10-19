// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// echo writes its arguments separated by blanks and terminated by a newline on
// the standard output.
//
// Synopsis:
//
//	echo [-e] [-n] [-E] [STRING]...
package echo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/u-root/u-root/pkg/core"
)

var usage = `echo:
  If -e is in effect, the following sequences are recognized:
    \\     backslash
    \a     alert (BEL)
    \b     backspace
    \c     produce no further output
    \f     form feed
    \n     new line
    \r     carriage return
    \t     horizontal tab
    \v     vertical tab
    \0NNN  byte with octal value NNN (1 to 3 digits)
    \xHH   byte with hexadecimal value HH (1 to 2 digits)`

func escapeString(s string) (string, error) {
	if len(s) < 1 {
		return "", nil
	}

	s = strings.Split(s, "\\c")[0]

	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			buf.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case '\\':
			buf.WriteByte('\\')
		case 'a':
			buf.WriteByte('\a')
		case 'b':
			buf.WriteByte('\b')
		case 'f':
			buf.WriteByte('\f')
		case 'n':
			buf.WriteByte('\n')
		case 'r':
			buf.WriteByte('\r')
		case 't':
			buf.WriteByte('\t')
		case 'v':
			buf.WriteByte('\v')
		case '0':
			var val byte
			digits := 0
			for digits < 3 && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7' {
				i++
				val = val*8 + (s[i] - '0')
				digits++
			}
			buf.WriteByte(val)
		case 'x':
			var val byte
			digits := 0
			for digits < 2 && i+1 < len(s) {
				c := s[i+1]
				if c >= '0' && c <= '9' {
					i++
					val = val*16 + (c - '0')
					digits++
				} else if c >= 'a' && c <= 'f' {
					i++
					val = val*16 + (c - 'a' + 10)
					digits++
				} else if c >= 'A' && c <= 'F' {
					i++
					val = val*16 + (c - 'A' + 10)
					digits++
				} else {
					break
				}
			}
			if digits > 0 {
				buf.WriteByte(val)
			} else {
				buf.WriteString("\\x")
			}
		default:
			buf.WriteByte('\\')
			buf.WriteByte(s[i])
		}
	}
	return buf.String(), nil
}

func echo(w io.Writer, noNewline, escape, backslash bool, s ...string) error {
	var err error
	if backslash {
		escape = false
	}
	line := strings.Join(s, " ")
	if escape {
		line, err = escapeString(line)
		if err != nil {
			return err
		}
	}
	format := "%s"
	if !noNewline {
		format += "\n"
	}
	_, err = fmt.Fprintf(w, format, line)

	return err
}

// Main executes the echo command with the given execution context.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "echo: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var (
		noNewline                 bool
		interpretEscapes          bool
		interpretBackslashEscapes bool
	)

	fs := flag.NewFlagSet("echo", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&noNewline, "n", false, "suppress newline")
	fs.BoolVar(&interpretEscapes, "e", true, "enable interpretation of backslash escapes (default)")
	fs.BoolVar(&interpretBackslashEscapes, "E", false, "disable interpretation of backslash escapes")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: echo [-e] [-n] [-E] [STRING]...\n\n%s\n\nOptions:\n", usage)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	select {
	case <-ctx.Done():
		return 130
	default:
	}

	if err := echo(ioEnv.Stdout, noNewline, interpretEscapes, interpretBackslashEscapes, fs.Args()...); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "echo: %v\n", err)
		return 1
	}
	return 0
}
