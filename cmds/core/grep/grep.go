// Copyright 2012-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// grep searches file contents using regular expressions.
//
// Synopsis:
//
//	grep [-clFivnhqre] [FILE]...
//
// Options:
//
//  -c, --count                Just show counts
//  -l, --files-with-matches   list only files
//  -F, --fixed-strings        Match using fixed strings
//  -i, --ignore-case          case-insensitive matching
//  -v, --invert-match         Print only non-matching lines
//  -n, --line-number          Show line numbers
//  -h, --no-filename          Suppress file name prefixes on output
//  -q, --quiet                Don't print matches; exit on first match
//  -r, --recursive            recursive
//  -e, --regexp string        Pattern to match

package grep

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
	"regexp"
	"strconv"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var errQuiet = fmt.Errorf("not found")

type params struct {
	expr string
	headers, invert, recursive, caseInsensitive, fixed,
	noShowMatch, quiet, count, number bool
}

type grepInput struct {
	r    io.Reader
	name string
}

type cmd struct {
	ctx        context.Context
	stdin      io.Reader
	stdout     *bufio.Writer
	stderr     io.Writer
	resolve    func(string) string
	args       []string
	params     params
	matchCount int
	matchedAny bool
	showName   bool
}

func (c *cmd) grepStream(in *grepInput, re *regexp.Regexp) (stopQuiet bool, err error) {
	scanner := bufio.NewScanner(in.r)
	lineNum := 0
	for scanner.Scan() {
		if c.ctx != nil {
			select {
			case <-c.ctx.Done():
				return false, c.ctx.Err()
			default:
			}
		}
		line := scanner.Text()
		var m bool
		switch {
		case c.params.fixed && c.params.caseInsensitive:
			m = strings.Contains(strings.ToLower(line), strings.ToLower(c.params.expr))
		case c.params.fixed && !c.params.caseInsensitive:
			m = strings.Contains(line, c.params.expr)
		default:
			m = re.MatchString(line)
		}
		if m != c.params.invert {
			c.matchedAny = true
			if c.params.quiet {
				return true, nil
			}
			c.printMatch(in, line, lineNum+1, m)
			if c.params.noShowMatch {
				break
			}
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, c.stdout.Flush()
}

func (c *cmd) printMatch(in *grepInput, line string, lineNum int, match bool) {
	if match == !c.params.invert {
		c.matchCount++
	}
	if c.params.count {
		return
	}
	defer func() {
		_ = c.stdout.WriteByte('\n')
	}()
	if c.showName {
		_, _ = c.stdout.WriteString(in.name)
	}
	if c.params.noShowMatch {
		return
	}
	if match == !c.params.invert {
		if c.showName {
			_ = c.stdout.WriteByte(':')
		}
		if c.params.number {
			_, _ = c.stdout.Write(strconv.AppendUint(nil, uint64(lineNum), 10))
			_ = c.stdout.WriteByte(':')
		}
		_, _ = c.stdout.WriteString(line)
	}
}

func (c *cmd) execute() error {
	defer c.stdout.Flush()

	if c.params.expr != "" {
		c.args = append([]string{c.params.expr}, c.args...)
	}
	r := ".*"
	if len(c.args) > 0 {
		r = c.args[0]
	}
	if c.params.caseInsensitive && !bytes.HasPrefix([]byte(r), []byte("(?i)")) && !c.params.fixed {
		r = "(?i)" + r
	}
	var re *regexp.Regexp
	if !c.params.fixed {
		var err error
		re, err = regexp.Compile(r)
		if err != nil {
			return err
		}
	} else if c.params.expr == "" && len(c.args) > 0 {
		c.params.expr = c.args[0]
	}

	if len(c.args) < 2 {
		r := c.stdin
		if r == nil {
			r = strings.NewReader("")
		}
		stop, err := c.grepStream(&grepInput{r: r, name: "<stdin>"}, re)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	} else {
		c.showName = (len(c.args[1:]) > 1 || c.params.recursive || c.params.noShowMatch) && !c.params.headers
		for _, v := range c.args[1:] {
			resolvedRoot := v
			if c.resolve != nil {
				resolvedRoot = c.resolve(v)
			}
			stop := false
			err := filepath.Walk(resolvedRoot, func(path string, fi os.FileInfo, err error) error {
				if c.ctx != nil {
					select {
					case <-c.ctx.Done():
						return c.ctx.Err()
					default:
					}
				}
				displayName := v
				if path != resolvedRoot {
					if rel, rerr := filepath.Rel(resolvedRoot, path); rerr == nil {
						displayName = filepath.Join(v, rel)
					} else {
						displayName = path
					}
				}
				if err != nil {
					fmt.Fprintf(c.stderr, "grep: %v: %v\n", displayName, err)
					return nil
				}
				if fi.IsDir() {
					if !c.params.recursive {
						fmt.Fprintf(c.stderr, "grep: %v: Is a directory\n", displayName)
						return filepath.SkipDir
					}
					return nil
				}
				fp, err := os.Open(path)
				if err != nil {
					fmt.Fprintf(c.stderr, "can't open %s: %v\n", displayName, err)
					return nil
				}
				defer fp.Close()
				stopFile, gerr := c.grepStream(&grepInput{r: fp, name: displayName}, re)
				if gerr != nil {
					return gerr
				}
				if stopFile {
					stop = true
					return filepath.SkipAll
				}
				return nil
			})
			if stop {
				return nil
			}
			if err != nil {
				return err
			}
		}
	}

	if c.params.quiet && !c.matchedAny {
		return errQuiet
	}
	if c.params.count {
		_, _ = c.stdout.Write(strconv.AppendUint(nil, uint64(c.matchCount), 10))
		_ = c.stdout.WriteByte('\n')
		_ = c.stdout.Flush()
	}
	return nil
}

func (c *cmd) run() error {
	return c.execute()
}

// run is the legacy test runner kept for backward compatibility with grep_test.go.
func run(stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, args []string) error {
	st := cmd{
		stdin:  stdin,
		stdout: bufio.NewWriter(stdout),
		stderr: stderr,
	}

	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.StringVar(&st.params.expr, "regexp", "", "Pattern to match")
	f.StringVar(&st.params.expr, "e", "", "Pattern to match (shorthand)")

	f.BoolVar(&st.params.headers, "no-filename", false, "Suppress file name prefixes on output")
	f.BoolVar(&st.params.headers, "h", false, "Suppress file name prefixes on output (shorthand)")

	f.BoolVar(&st.params.invert, "invert-match", false, "Print only non-matching lines")
	f.BoolVar(&st.params.invert, "v", false, "Print only non-matching lines (shorthand)")

	f.BoolVar(&st.params.recursive, "recursive", false, "recursive")
	f.BoolVar(&st.params.recursive, "r", false, "recursive")

	f.BoolVar(&st.params.noShowMatch, "files-with-matches", false, "list only files")
	f.BoolVar(&st.params.noShowMatch, "l", false, "list only files (shorthand)")

	f.BoolVar(&st.params.count, "count", false, "Just show counts")
	f.BoolVar(&st.params.count, "c", false, "Just show counts")

	f.BoolVar(&st.params.caseInsensitive, "ignore-case", false, "case-insensitive matching")
	f.BoolVar(&st.params.caseInsensitive, "i", false, "case-insensitive matching (shorthand)")

	f.BoolVar(&st.params.number, "line-number", false, "Show line numbers")
	f.BoolVar(&st.params.number, "n", false, "Show line numbers (shorthand)")

	f.BoolVar(&st.params.fixed, "fixed-strings", false, "Match using fixed strings")
	f.BoolVar(&st.params.fixed, "F", false, "Match using fixed strings (shorthand)")

	f.BoolVar(&st.params.quiet, "quiet", false, "Don't print matches; exit on first match")
	f.BoolVar(&st.params.quiet, "q", false, "Don't print matches; exit on first match (shorthand)")

	f.BoolVar(&st.params.quiet, "silent", false, "Don't print matches; exit on first match")
	f.BoolVar(&st.params.quiet, "s", false, "Don't print matches; exit on first match (shorthand)")

	f.Usage = func() {
		fmt.Fprint(f.Output(), "Usage: grep [-clFivnhqre] [FILE]...\n\n")
		f.PrintDefaults()
	}

	if err := f.Parse(unixflag.ArgsToGoArgs(args[1:])); err != nil {
		return err
	}

	st.args = f.Args()
	return st.execute()
}

// Main executes the grep command with the given execution context.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "grep: internal panic: %v\n", r)
			exitCode = 2
		}
	}()

	st := cmd{
		ctx:     ctx,
		stdin:   ioEnv.Stdin,
		stdout:  bufio.NewWriter(ioEnv.Stdout),
		stderr:  ioEnv.Stderr,
		resolve: ioEnv.ResolvePath,
	}

	f := flag.NewFlagSet("grep", flag.ContinueOnError)
	f.SetOutput(ioEnv.Stderr)

	f.StringVar(&st.params.expr, "regexp", "", "Pattern to match")
	f.StringVar(&st.params.expr, "e", "", "Pattern to match (shorthand)")

	f.BoolVar(&st.params.headers, "no-filename", false, "Suppress file name prefixes on output")
	f.BoolVar(&st.params.headers, "h", false, "Suppress file name prefixes on output (shorthand)")

	f.BoolVar(&st.params.invert, "invert-match", false, "Print only non-matching lines")
	f.BoolVar(&st.params.invert, "v", false, "Print only non-matching lines (shorthand)")

	f.BoolVar(&st.params.recursive, "recursive", false, "recursive")
	f.BoolVar(&st.params.recursive, "r", false, "recursive")

	f.BoolVar(&st.params.noShowMatch, "files-with-matches", false, "list only files")
	f.BoolVar(&st.params.noShowMatch, "l", false, "list only files (shorthand)")

	f.BoolVar(&st.params.count, "count", false, "Just show counts")
	f.BoolVar(&st.params.count, "c", false, "Just show counts")

	f.BoolVar(&st.params.caseInsensitive, "ignore-case", false, "case-insensitive matching")
	f.BoolVar(&st.params.caseInsensitive, "i", false, "case-insensitive matching (shorthand)")

	f.BoolVar(&st.params.number, "line-number", false, "Show line numbers")
	f.BoolVar(&st.params.number, "n", false, "Show line numbers (shorthand)")

	f.BoolVar(&st.params.fixed, "fixed-strings", false, "Match using fixed strings")
	f.BoolVar(&st.params.fixed, "F", false, "Match using fixed strings (shorthand)")

	f.BoolVar(&st.params.quiet, "quiet", false, "Don't print matches; exit on first match")
	f.BoolVar(&st.params.quiet, "q", false, "Don't print matches; exit on first match (shorthand)")
	f.BoolVar(&st.params.quiet, "silent", false, "Don't print matches; exit on first match")
	f.BoolVar(&st.params.quiet, "s", false, "Don't print matches; exit on first match (shorthand)")

	f.Usage = func() {
		fmt.Fprint(f.Output(), "Usage: grep [-clFivnhqre] [FILE]...\n\n")
		f.PrintDefaults()
	}

	if err := f.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	st.args = f.Args()
	if err := st.execute(); err != nil {
		if errors.Is(err, errQuiet) {
			return 1
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "grep: %v\n", err)
		return 2
	}

	if !st.matchedAny {
		return 1
	}
	return 0
}
