// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package grep implements a reentrant grep command for u-root.
package grep

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/u-root/u-root/cmds/core/search"
	"github.com/u-root/u-root/pkg/core"
)

var (
	errHelp = errors.New("help requested")
	errStop = errors.New("quiet match")
)

type options struct {
	basic, fixed, perl, ignoreCase, invert, word, wholeLine bool
	count, listMatches, listNoMatches, quiet, silent        bool
	lineNumber, byteOffset, withFilename, noFilename        bool
	recursive, followLinks, onlyMatching, nullData          bool
	nullFilename, text, lineBuffered, binaryWithoutMatch    bool
	maxCount, before, after                                 int
	color                                                   string
	include, exclude, excludeDir                            []string
}

type record struct {
	data   []byte
	line   int64
	offset int64
}

type command struct {
	ctx      context.Context
	ioEnv    core.IO
	opts     options
	matcher  *search.Matcher
	stdin    io.Reader
	stdout   *bufio.Writer
	errOut   io.Writer
	showName bool
	found    bool
	hadError bool
	color    bool
	visited  map[string]struct{}
}

func Main(ctx context.Context, ioEnv core.IO, args ...string) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if ioEnv.Stdin == nil {
		ioEnv.Stdin = os.Stdin
	}
	if ioEnv.Stdout == nil {
		ioEnv.Stdout = os.Stdout
	}
	if ioEnv.Stderr == nil {
		ioEnv.Stderr = os.Stderr
	}

	opts, patterns, paths, stdin, err := parseArgs(args, ioEnv)
	if errors.Is(err, errHelp) {
		_, _ = io.WriteString(ioEnv.Stderr, usage)
		return 0
	}
	if err != nil {
		if !opts.silent {
			fmt.Fprintf(ioEnv.Stderr, "grep: %v\n", err)
		}
		return 2
	}
	matcher, err := search.Compile(patterns, search.Options{
		Basic:      opts.basic,
		Fixed:      opts.fixed,
		IgnoreCase: opts.ignoreCase,
		Perl:       opts.perl,
		WholeWord:  opts.word,
		WholeLine:  opts.wholeLine,
	})
	if err != nil && len(patterns) != 0 {
		fmt.Fprintf(ioEnv.Stderr, "grep: %v\n", err)
		return 2
	}
	if len(paths) == 0 {
		paths = []string{"-"}
	}

	cmd := &command{
		ctx:      ctx,
		ioEnv:    ioEnv,
		opts:     opts,
		matcher:  matcher,
		stdin:    stdin,
		stdout:   bufio.NewWriter(ioEnv.Stdout),
		errOut:   ioEnv.Stderr,
		showName: !opts.noFilename && (opts.withFilename || len(paths) > 1 || opts.recursive),
		visited:  make(map[string]struct{}),
	}
	cmd.color = opts.color == "always" ||
		(opts.color == "auto" && ioEnv.StdoutIsTerminal && ioEnv.Getenv("NO_COLOR") == "")

	err = cmd.run(paths)
	if flushErr := cmd.stdout.Flush(); err == nil {
		err = flushErr
	}
	if errors.Is(err, errStop) {
		return 0
	}
	if err != nil {
		if isBrokenPipe(err) {
			return 0
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "grep: %v\n", err)
		return 2
	}
	if cmd.hadError {
		return 2
	}
	if cmd.found {
		return 0
	}
	return 1
}

const usage = `Usage: grep [OPTION]... PATTERNS [FILE]...
Search for PATTERNS in each FILE or standard input.

Matching control:
  -E, --extended-regexp     use extended regular expressions
  -F, --fixed-strings       use fixed strings
  -G, --basic-regexp        use basic regular expressions (default)
  -P, --perl-regexp         use Perl-compatible regular expressions
  -e, --regexp=PATTERN      use PATTERN; may be repeated
  -f, --file=FILE           read patterns from FILE
  -i, --ignore-case         ignore case distinctions
  -v, --invert-match        select non-matching lines
  -w, --word-regexp         match whole words
  -x, --line-regexp         match whole lines

Output control:
  -c, --count               print selected line counts
  -l, --files-with-matches  print only names of matching files
  -L, --files-without-match print only names of non-matching files
  -n, --line-number         print line numbers
  -b, --byte-offset         print byte offsets
  -o, --only-matching       print only matching parts
  -q, --quiet               suppress normal output
  -H, --with-filename       print file names
  -h, --no-filename         suppress file names
  -A, -B, -C N              print trailing, leading, or surrounding context
  -r, --recursive           search directories recursively
  -R                        follow symbolic links while recursing
  -z, --null-data           use NUL as the record delimiter
  -Z, --null                separate file names with NUL
      --include=GLOB        search only files matching GLOB
      --exclude=GLOB        skip files matching GLOB
      --exclude-dir=GLOB    skip directories matching GLOB
      --color=WHEN          colorize matching text (always, never, auto)
`

func parseArgs(args []string, ioEnv core.IO) (options, []string, []string, io.Reader, error) {
	opts := options{basic: true, maxCount: -1, color: "never"}
	patterns := make([]string, 0)
	var positional []string
	stdin := ioEnv.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdinConsumed := false
	patternSpecified := false
	endOptions := false

	addPatternFile := func(name string) error {
		var data []byte
		if name == "-" {
			var err error
			data, err = io.ReadAll(stdin)
			if err != nil {
				return fmt.Errorf("read patterns from standard input: %w", err)
			}
			stdinConsumed = true
			stdin = bytes.NewReader(nil)
		} else {
			var err error
			data, err = os.ReadFile(ioEnv.ResolvePath(name))
			if err != nil {
				return fmt.Errorf("read pattern file %q: %w", name, err)
			}
		}
		if len(data) == 0 {
			return nil
		}
		lines := bytes.Split(data, []byte{'\n'})
		if len(lines) > 0 && len(lines[len(lines)-1]) == 0 && data[len(data)-1] == '\n' {
			lines = lines[:len(lines)-1]
		}
		for _, line := range lines {
			patterns = append(patterns, string(line))
		}
		return nil
	}

	value := func(arg string, index *int, name string) (string, error) {
		if arg != "" {
			return arg, nil
		}
		*index++
		if *index >= len(args) {
			return "", fmt.Errorf("%s requires an argument", name)
		}
		return args[*index], nil
	}
	parseNumber := func(v, name string) (int, error) {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid %s value %q", name, v)
		}
		return n, nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !endOptions && arg == "--" {
			endOptions = true
			continue
		}
		if !endOptions && (arg == "--help" || arg == "-h" && i == 0 && len(args) == 1) {
			return opts, nil, nil, stdin, errHelp
		}
		if !endOptions && strings.HasPrefix(arg, "--") {
			name, val, hasValue := strings.Cut(arg, "=")
			switch name {
			case "--help":
				return opts, nil, nil, stdin, errHelp
			case "--regexp":
				v, err := value(func() string {
					if hasValue {
						return val
					}
					return ""
				}(), &i, name)
				if err != nil {
					return opts, nil, nil, stdin, err
				}
				patternSpecified = true
				patterns = append(patterns, v)
			case "--file":
				v, err := value(func() string {
					if hasValue {
						return val
					}
					return ""
				}(), &i, name)
				if err != nil {
					return opts, nil, nil, stdin, err
				}
				patternSpecified = true
				if err := addPatternFile(v); err != nil {
					return opts, nil, nil, stdin, err
				}
			case "--extended-regexp":
				opts.basic, opts.fixed, opts.perl = false, false, false
			case "--basic-regexp":
				opts.basic, opts.fixed, opts.perl = true, false, false
			case "--fixed-strings":
				opts.basic, opts.fixed, opts.perl = false, true, false
			case "--perl-regexp":
				opts.basic, opts.fixed, opts.perl = false, false, true
			case "--ignore-case":
				opts.ignoreCase = true
			case "--invert-match":
				opts.invert = true
			case "--word-regexp":
				opts.word = true
			case "--line-regexp":
				opts.wholeLine = true
			case "--count":
				opts.count = true
			case "--files-with-matches":
				opts.listMatches = true
			case "--files-without-match":
				opts.listNoMatches = true
			case "--quiet", "--silent":
				opts.quiet = true
				opts.silent = opts.silent || name == "--silent"
			case "--line-number":
				opts.lineNumber = true
			case "--byte-offset":
				opts.byteOffset = true
			case "--only-matching":
				opts.onlyMatching = true
			case "--with-filename":
				opts.withFilename, opts.noFilename = true, false
			case "--no-filename":
				opts.noFilename, opts.withFilename = true, false
			case "--recursive":
				opts.recursive = true
			case "--null":
				opts.nullFilename = true
			case "--null-data":
				opts.nullData = true
			case "--text":
				opts.text = true
				opts.binaryWithoutMatch = false
			case "--binary-files":
				v, err := value(func() string {
					if hasValue {
						return val
					}
					return ""
				}(), &i, name)
				if err != nil {
					return opts, nil, nil, stdin, err
				}
				switch v {
				case "text":
					opts.text, opts.binaryWithoutMatch = true, false
				case "without-match":
					opts.text, opts.binaryWithoutMatch = false, true
				case "binary":
					opts.text, opts.binaryWithoutMatch = false, false
				default:
					return opts, nil, nil, stdin, fmt.Errorf("invalid --binary-files mode %q", v)
				}
			case "--line-buffered":
				opts.lineBuffered = true
			case "--max-count":
				v, err := value(func() string {
					if hasValue {
						return val
					}
					return ""
				}(), &i, name)
				if err != nil {
					return opts, nil, nil, stdin, err
				}
				opts.maxCount, err = parseNumber(v, "max-count")
				if err != nil {
					return opts, nil, nil, stdin, err
				}
			case "--after-context", "--before-context", "--context":
				v, err := value(func() string {
					if hasValue {
						return val
					}
					return ""
				}(), &i, name)
				if err != nil {
					return opts, nil, nil, stdin, err
				}
				n, err := parseNumber(v, "context")
				if err != nil {
					return opts, nil, nil, stdin, err
				}
				switch name {
				case "--after-context":
					opts.after = n
				case "--before-context":
					opts.before = n
				default:
					opts.before, opts.after = n, n
				}
			case "--include", "--exclude", "--exclude-dir":
				v, err := value(func() string {
					if hasValue {
						return val
					}
					return ""
				}(), &i, name)
				if err != nil {
					return opts, nil, nil, stdin, err
				}
				switch name {
				case "--include":
					opts.include = append(opts.include, v)
				case "--exclude":
					opts.exclude = append(opts.exclude, v)
				default:
					opts.excludeDir = append(opts.excludeDir, v)
				}
			case "--color", "--colour":
				opts.color = "auto"
				if hasValue {
					opts.color = val
				}
				if opts.color != "always" && opts.color != "never" && opts.color != "auto" {
					return opts, nil, nil, stdin, fmt.Errorf("invalid color mode %q", opts.color)
				}
			default:
				return opts, nil, nil, stdin, fmt.Errorf("unknown option %q", arg)
			}
			continue
		}
		if !endOptions && strings.HasPrefix(arg, "-") && arg != "-" {
			for j := 1; j < len(arg); j++ {
				flag := arg[j]
				attached := arg[j+1:]
				switch flag {
				case 'e', 'f', 'A', 'B', 'C', 'm':
					v, err := value(attached, &i, "-"+string(flag))
					if err != nil {
						return opts, nil, nil, stdin, err
					}
					j = len(arg)
					switch flag {
					case 'e':
						patternSpecified = true
						patterns = append(patterns, v)
					case 'f':
						patternSpecified = true
						if err := addPatternFile(v); err != nil {
							return opts, nil, nil, stdin, err
						}
					case 'm':
						opts.maxCount, err = parseNumber(v, "max-count")
						if err != nil {
							return opts, nil, nil, stdin, err
						}
					default:
						n, err := parseNumber(v, "context")
						if err != nil {
							return opts, nil, nil, stdin, err
						}
						switch flag {
						case 'A':
							opts.after = n
						case 'B':
							opts.before = n
						case 'C':
							opts.before, opts.after = n, n
						}
					}
				default:
					switch flag {
					case 'E':
						opts.basic, opts.fixed, opts.perl = false, false, false
					case 'F':
						opts.basic, opts.fixed, opts.perl = false, true, false
					case 'G':
						opts.basic, opts.fixed, opts.perl = true, false, false
					case 'P':
						opts.basic, opts.fixed, opts.perl = false, false, true
					case 'i':
						opts.ignoreCase = true
					case 'v':
						opts.invert = true
					case 'w':
						opts.word = true
					case 'x':
						opts.wholeLine = true
					case 'c':
						opts.count = true
					case 'l':
						opts.listMatches = true
					case 'L':
						opts.listNoMatches = true
					case 'q':
						opts.quiet = true
					case 's':
						opts.silent = true
					case 'n':
						opts.lineNumber = true
					case 'b':
						opts.byteOffset = true
					case 'o':
						opts.onlyMatching = true
					case 'H':
						opts.withFilename, opts.noFilename = true, false
					case 'h':
						opts.noFilename, opts.withFilename = true, false
					case 'r':
						opts.recursive = true
					case 'R':
						opts.recursive, opts.followLinks = true, true
					case 'Z':
						opts.nullFilename = true
					case 'z':
						opts.nullData = true
					case 'a':
						opts.text = true
						opts.binaryWithoutMatch = false
					case 'I':
						opts.text, opts.binaryWithoutMatch = false, true
					default:
						return opts, nil, nil, stdin, fmt.Errorf("unknown option -%c", flag)
					}
				}
			}
			continue
		}
		positional = append(positional, arg)
	}

	if !patternSpecified {
		if len(positional) == 0 {
			return opts, nil, nil, stdin, fmt.Errorf("missing pattern")
		}
		patterns = append(patterns, positional[0])
		positional = positional[1:]
	}
	if stdinConsumed && len(positional) == 0 {
		stdin = bytes.NewReader(nil)
	}
	return opts, patterns, positional, stdin, nil
}

func (c *command) run(paths []string) error {
	for _, path := range paths {
		if err := c.checkContext(); err != nil {
			return err
		}
		if path == "-" {
			name := "(standard input)"
			if err := c.scanReader(c.stdin, name); err != nil {
				if errors.Is(err, errStop) {
					return err
				}
				c.report(name, err)
			}
			continue
		}
		if err := c.searchPath(c.ioEnv.ResolvePath(path), path, false); err != nil {
			if errors.Is(err, errStop) {
				return err
			}
			c.report(path, err)
		}
	}
	return nil
}

func (c *command) searchPath(path, display string, fromWalk bool) error {
	if err := c.checkContext(); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if fromWalk && !c.opts.followLinks {
			return nil
		}
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		info, err = os.Stat(target)
		if err != nil {
			return err
		}
		path = target
	}
	if info.IsDir() {
		if !c.opts.recursive {
			return fmt.Errorf("Is a directory")
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		real, err = filepath.Abs(real)
		if err != nil {
			return err
		}
		if _, seen := c.visited[real]; seen {
			return nil
		}
		c.visited[real] = struct{}{}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			childDisplay := filepath.Join(display, entry.Name())
			childPath := filepath.Join(path, entry.Name())
			childInfo, err := os.Lstat(childPath)
			if err != nil {
				c.report(childDisplay, err)
				continue
			}
			if childInfo.IsDir() && c.matchesAny(c.opts.excludeDir, childDisplay) {
				continue
			}
			if c.shouldSkipFile(childDisplay) {
				continue
			}
			if err := c.searchPath(childPath, childDisplay, true); err != nil {
				if errors.Is(err, errStop) {
					return err
				}
				c.report(childDisplay, err)
			}
		}
		return nil
	}
	if c.shouldSkipFile(display) || !info.Mode().IsRegular() {
		return nil
	}
	fp, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fp.Close()
	if err := c.scanReader(fp, display); err != nil {
		return err
	}
	return nil
}

func (c *command) scanReader(input io.Reader, name string) error {
	if input == nil {
		input = bytes.NewReader(nil)
	}
	if c.opts.maxCount == 0 {
		if c.opts.count {
			return c.writeCount(name, 0)
		}
		if c.opts.listNoMatches {
			c.found = true
			return c.writeFileName(name)
		}
		return nil
	}
	if !c.opts.text && !c.opts.nullData {
		var binary bool
		var err error
		input, binary, err = detectBinary(c.ctx, input)
		if err != nil {
			return err
		}
		if binary {
			return c.scanBinary(input, name)
		}
	}
	reader := bufio.NewReader(input)
	delim := byte('\n')
	if c.opts.nullData {
		delim = 0
	}
	var (
		offset         int64
		lineNumber     int64
		before         []record
		afterRemaining int
		selectedCount  int
		lastOutputLine int64
		fileMatched    bool
		stoppedMatch   bool
	)
	for {
		if err := c.checkContext(); err != nil {
			return err
		}
		data, readErr := reader.ReadBytes(delim)
		if len(data) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		if len(data) == 0 && readErr != nil {
			return readErr
		}
		terminated := data[len(data)-1] == delim
		if terminated {
			data = data[:len(data)-1]
		}
		current := record{data: data, line: lineNumber + 1, offset: offset}
		lineNumber++
		if terminated {
			offset += int64(len(data)) + 1
		} else {
			offset += int64(len(data))
		}

		matches := [][]int(nil)
		selected := false
		if !stoppedMatch {
			if c.matcher != nil {
				matches = c.matcher.FindAllIndex(data, 0)
			}
			selected = len(matches) > 0
			if c.opts.invert {
				selected = !selected
			}
		}
		if selected {
			fileMatched = true
			selectedCount++
			c.found = true
			if c.opts.quiet {
				return errStop
			}
			if c.opts.listMatches {
				if err := c.writeFileName(name); err != nil {
					return err
				}
				return nil
			}
			if !c.opts.count {
				for _, old := range before {
					if err := c.emit(name, old, false, nil, &lastOutputLine); err != nil {
						return err
					}
				}
				before = before[:0]
				if err := c.emit(name, current, true, matches, &lastOutputLine); err != nil {
					return err
				}
				afterRemaining = c.opts.after
			}
			if c.opts.maxCount > 0 && selectedCount >= c.opts.maxCount {
				stoppedMatch = true
			}
		} else if afterRemaining > 0 && !c.opts.count && !c.opts.listNoMatches {
			if err := c.emit(name, current, false, nil, &lastOutputLine); err != nil {
				return err
			}
			afterRemaining--
		} else if c.opts.before > 0 && !c.opts.count && !c.opts.listNoMatches {
			before = append(before, current)
			if len(before) > c.opts.before {
				copy(before, before[len(before)-c.opts.before:])
				before = before[:c.opts.before]
			}
		}
		if stoppedMatch && afterRemaining == 0 {
			break
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return readErr
			}
			break
		}
	}
	if c.opts.count {
		if err := c.writeCount(name, selectedCount); err != nil {
			return err
		}
	}
	if c.opts.listNoMatches && !fileMatched {
		c.found = true
		return c.writeFileName(name)
	}
	return nil
}

func detectBinary(ctx context.Context, input io.Reader) (io.Reader, bool, error) {
	if seeker, ok := input.(io.ReadSeeker); ok {
		position, err := seeker.Seek(0, io.SeekCurrent)
		if err == nil {
			var buf [32 * 1024]byte
			binary := false
			for {
				if err := ctx.Err(); err != nil {
					_, _ = seeker.Seek(position, io.SeekStart)
					return nil, false, err
				}
				n, readErr := seeker.Read(buf[:])
				if bytes.IndexByte(buf[:n], 0) >= 0 {
					binary = true
					break
				}
				if readErr != nil {
					if !errors.Is(readErr, io.EOF) {
						return nil, false, readErr
					}
					break
				}
				if n == 0 {
					return nil, false, io.ErrNoProgress
				}
			}
			if _, err := seeker.Seek(position, io.SeekStart); err != nil {
				return nil, false, err
			}
			return input, binary, nil
		}
	}
	var data bytes.Buffer
	var buf [32 * 1024]byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		n, readErr := input.Read(buf[:])
		if n > 0 {
			if _, err := data.Write(buf[:n]); err != nil {
				return nil, false, err
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return nil, false, readErr
			}
			break
		}
		if n == 0 {
			return nil, false, io.ErrNoProgress
		}
	}
	result := data.Bytes()
	return bytes.NewReader(result), bytes.IndexByte(result, 0) >= 0, nil
}

func (c *command) scanBinary(input io.Reader, name string) error {
	if c.opts.maxCount == 0 {
		if c.opts.count {
			return c.writeCount(name, 0)
		}
		if c.opts.listNoMatches {
			c.found = true
			return c.writeFileName(name)
		}
		return nil
	}
	if c.opts.binaryWithoutMatch {
		if c.opts.count {
			return c.writeCount(name, 0)
		}
		if c.opts.listNoMatches {
			c.found = true
			return c.writeFileName(name)
		}
		return nil
	}
	reader := bufio.NewReader(input)
	delim := byte('\n')
	if c.opts.nullData {
		delim = 0
	}
	selected := 0
	for {
		if err := c.checkContext(); err != nil {
			return err
		}
		record, err := reader.ReadBytes(delim)
		if len(record) == 0 && errors.Is(err, io.EOF) {
			break
		}
		if len(record) > 0 && record[len(record)-1] == delim {
			record = record[:len(record)-1]
		}
		matches := c.matcher.FindAllIndex(record, 0)
		match := len(matches) > 0
		if c.opts.invert {
			match = !match
		}
		if match {
			selected++
			if c.opts.maxCount > 0 && selected >= c.opts.maxCount {
				break
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
			break
		}
	}
	if c.opts.count {
		if err := c.writeCount(name, selected); err != nil {
			return err
		}
	}
	if selected == 0 {
		if c.opts.listNoMatches {
			c.found = true
			return c.writeFileName(name)
		}
		return nil
	}
	c.found = true
	if c.opts.quiet {
		return errStop
	}
	if c.opts.listMatches {
		return c.writeFileName(name)
	}
	if !c.opts.count {
		_, err := fmt.Fprintf(c.stdout, "Binary file %s matches\n", name)
		return err
	}
	return nil
}

func (c *command) writeCount(name string, count int) error {
	if c.showName {
		if _, err := c.stdout.WriteString(name); err != nil {
			return err
		}
		if err := c.stdout.WriteByte(':'); err != nil {
			return err
		}
	}
	if _, err := c.stdout.WriteString(strconv.Itoa(count)); err != nil {
		return err
	}
	if err := c.stdout.WriteByte(recordDelimiter(c.opts.nullData)); err != nil {
		return err
	}
	if c.opts.lineBuffered {
		return c.stdout.Flush()
	}
	return nil
}

func (c *command) emit(name string, rec record, match bool, matches [][]int, lastOutputLine *int64) error {
	if (c.opts.before > 0 || c.opts.after > 0) && *lastOutputLine != 0 && rec.line > *lastOutputLine+1 {
		if _, err := c.stdout.Write([]byte{'-', '-', recordDelimiter(c.opts.nullData)}); err != nil {
			return err
		}
	}
	separator := byte(':')
	if !match {
		separator = '-'
	}
	writePrefix := func(byteOffset int64) error {
		if c.showName {
			if _, err := c.stdout.WriteString(name); err != nil {
				return err
			}
			nameSeparator := separator
			if c.opts.nullFilename {
				nameSeparator = 0
			}
			if err := c.stdout.WriteByte(nameSeparator); err != nil {
				return err
			}
		}
		if c.opts.lineNumber {
			if _, err := c.stdout.WriteString(strconv.FormatInt(rec.line, 10)); err != nil {
				return err
			}
			if err := c.stdout.WriteByte(separator); err != nil {
				return err
			}
		}
		if c.opts.byteOffset {
			if _, err := c.stdout.WriteString(strconv.FormatInt(byteOffset, 10)); err != nil {
				return err
			}
			if err := c.stdout.WriteByte(separator); err != nil {
				return err
			}
		}
		return nil
	}

	if match && c.opts.onlyMatching && !c.opts.invert {
		for _, span := range matches {
			if err := writePrefix(rec.offset + int64(span[0])); err != nil {
				return err
			}
			if c.color {
				if _, err := c.stdout.WriteString("\x1b[01;31m"); err != nil {
					return err
				}
			}
			if _, err := c.stdout.Write(rec.data[span[0]:span[1]]); err != nil {
				return err
			}
			if c.color {
				if _, err := c.stdout.WriteString("\x1b[m"); err != nil {
					return err
				}
			}
			if err := c.stdout.WriteByte(recordDelimiter(c.opts.nullData)); err != nil {
				return err
			}
		}
	} else {
		if err := writePrefix(rec.offset); err != nil {
			return err
		}
		if c.color && match && !c.opts.invert {
			if err := c.writeHighlighted(rec.data, matches); err != nil {
				return err
			}
		} else if _, err := c.stdout.Write(rec.data); err != nil {
			return err
		}
		if err := c.stdout.WriteByte(recordDelimiter(c.opts.nullData)); err != nil {
			return err
		}
	}
	*lastOutputLine = rec.line
	if c.opts.lineBuffered {
		return c.stdout.Flush()
	}
	return nil
}

func (c *command) writeHighlighted(data []byte, matches [][]int) error {
	offset := 0
	for _, match := range matches {
		if match[0] < offset {
			continue
		}
		if _, err := c.stdout.Write(data[offset:match[0]]); err != nil {
			return err
		}
		if _, err := c.stdout.WriteString("\x1b[01;31m"); err != nil {
			return err
		}
		if _, err := c.stdout.Write(data[match[0]:match[1]]); err != nil {
			return err
		}
		if _, err := c.stdout.WriteString("\x1b[m"); err != nil {
			return err
		}
		offset = match[1]
	}
	_, err := c.stdout.Write(data[offset:])
	return err
}

func recordDelimiter(nullData bool) byte {
	if nullData {
		return 0
	}
	return '\n'
}

func (c *command) writeFileName(name string) error {
	if _, err := c.stdout.WriteString(name); err != nil {
		return err
	}

	delim := byte('\n')
	if c.opts.nullFilename {
		delim = 0
	}
	return c.stdout.WriteByte(delim)
}

func (c *command) shouldSkipFile(name string) bool {
	if len(c.opts.include) != 0 && !c.matchesAny(c.opts.include, name) {
		return true
	}
	return c.matchesAny(c.opts.exclude, name)
}

func (c *command) matchesAny(patterns []string, name string) bool {
	base := filepath.Base(name)
	normalized := filepath.ToSlash(name)
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(pattern)
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, normalized); ok {
			return true
		}
	}
	return false
}

func (c *command) checkContext() error {
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
		return nil
	}
}

func (c *command) report(name string, err error) {
	c.hadError = true
	if !c.opts.silent {
		fmt.Fprintf(c.errOut, "grep: %s: %v\n", name, err)
	}
}

func isBrokenPipe(err error) bool {
	return errors.Is(err, io.ErrClosedPipe) || errors.Is(err, syscall.EPIPE)
}
