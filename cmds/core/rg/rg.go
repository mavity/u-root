// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rg implements an in-process recursive search command.
package rg

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	"github.com/u-root/u-root/cmds/core/search"
	"github.com/u-root/u-root/pkg/core"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

type options struct {
	fixed, ignoreCase, invert, word, wholeLine, perl                        bool
	hidden, noIgnore, text, follow, nullData, multiline, searchZip          bool
	globCaseInsensitive, stats                                              bool
	lineNumber, lineNumberExplicit, column, withFilename, noFilename        bool
	count, countMatches, files, filesWithMatches, filesWithoutMatch         bool
	onlyMatching, quiet, json, noHeading, typeList, nullFilename, smartCase bool
	color, sort, contextSeparator                                           string
	maxCount, maxDepth, before, after                                       int
	maxFileSize                                                             int64
	globs, types, excludedTypes                                             []string
	patterns                                                                []string
	typeDefs                                                                map[string][]string
	stdin                                                                   io.Reader
}

type ignoreRule struct {
	base, pattern             string
	negate, dirOnly, anchored bool
}

type command struct {
	ctx              context.Context
	ioEnv            core.IO
	opts             options
	matcher          *search.Matcher
	out              *bufio.Writer
	errOut           io.Writer
	stdoutCounter    *countingWriter
	jsonBytesPrinted int64
	started          time.Time
	searchElapsed    time.Duration
	found, hadError  bool
	multiplePaths    bool
	color            bool
	seen             map[string]struct{}
	stats            searchStats
}

type searchStats struct {
	matches, matchedLines, filesWithMatches, filesSearched, bytesSearched, bytesPrinted int64
}

type countingWriter struct {
	writer  io.Writer
	written int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.written += int64(n)
	return n, err
}

var errStop = errors.New("quiet match")

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
	opts, paths, err := parseArgs(args, ioEnv)
	if errors.Is(err, errHelp) {
		_, _ = io.WriteString(ioEnv.Stdout, usage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", err)
		return 2
	}
	if opts.stdin != nil {
		ioEnv.Stdin = opts.stdin
	}
	if opts.json && (opts.files || opts.count || opts.countMatches || opts.filesWithMatches || opts.filesWithoutMatch) {
		fmt.Fprintln(ioEnv.Stderr, "rg: --json cannot be combined with file-listing or count output modes")
		return 2
	}
	stdoutCounter := &countingWriter{writer: ioEnv.Stdout}
	if !opts.files && !opts.typeList {
		matcher, err := search.Compile(opts.patterns, search.Options{
			Fixed: opts.fixed, IgnoreCase: opts.ignoreCase, Perl: opts.perl,
			WholeWord: opts.word, WholeLine: opts.wholeLine,
		})
		if err != nil {
			fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", err)
			return 2
		}
		cmd := &command{
			ctx: ctx, ioEnv: ioEnv, opts: opts, matcher: matcher,
			out: bufio.NewWriter(stdoutCounter), errOut: ioEnv.Stderr,
			stdoutCounter: stdoutCounter, started: time.Now(),
			seen: make(map[string]struct{}),
		}
		cmd.color = opts.color == "always" ||
			(opts.color == "auto" && ioEnv.StdoutIsTerminal && ioEnv.Getenv("NO_COLOR") == "")
		searchStarted := time.Now()
		runErr := cmd.run(paths)
		cmd.searchElapsed = time.Since(searchStarted)
		if runErr != nil && !errors.Is(runErr, errStop) {
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
				return 130
			}
			fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", runErr)
			return 2
		}
		if err := cmd.writeJSONSummary(); err != nil {
			fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", err)
			return 2
		}
		if err := cmd.out.Flush(); err != nil {
			if errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
				return 0
			}
			fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", err)
			return 2
		}
		if err := cmd.writeStats(); err != nil {
			if errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
				return 0
			}
			fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", err)
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
	cmd := &command{
		ctx: ctx, ioEnv: ioEnv, opts: opts, out: bufio.NewWriter(stdoutCounter), errOut: ioEnv.Stderr,
		stdoutCounter: stdoutCounter, started: time.Now(),
	}
	if opts.typeList {
		names := make([]string, 0, len(opts.typeDefs))
		for name := range opts.typeDefs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(cmd.out, "%s: %s\n", name, strings.Join(opts.typeDefs[name], ", "))
		}
		_ = cmd.out.Flush()
		return 0
	}
	searchStarted := time.Now()
	runErr := cmd.run(paths)
	cmd.searchElapsed = time.Since(searchStarted)
	if runErr != nil {
		fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", runErr)
		return 2
	}
	if err := cmd.writeJSONSummary(); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", err)
		return 2
	}
	if err := cmd.out.Flush(); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", err)
		return 2
	}
	if err := cmd.writeStats(); err != nil {
		if errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
			return 0
		}
		fmt.Fprintf(ioEnv.Stderr, "rg: %v\n", err)
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

const usage = `Usage: rg [OPTIONS] PATTERN [PATH ...]
Search recursively for a regular expression in files.
`

var errHelp = errors.New("help requested")

func parseArgs(args []string, ioEnv core.IO) (options, []string, error) {
	opts := options{
		color: "auto", sort: "path", contextSeparator: "--",
		maxCount: -1, maxDepth: -1, typeDefs: cloneTypes(),
	}
	var paths []string
	var positional []string
	endOptions := false
	value := func(arg string, i *int, name string) (string, error) {
		if arg != "" {
			return arg, nil
		}
		*i++
		if *i >= len(args) {
			return "", fmt.Errorf("%s requires an argument", name)
		}
		return args[*i], nil
	}
	number := func(s, name string) (int, error) {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid %s %q", name, s)
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
			return opts, nil, errHelp
		}
		if !endOptions && strings.HasPrefix(arg, "--") {
			name, val, hasValue := strings.Cut(arg, "=")
			getValue := func() (string, error) {
				if hasValue {
					return val, nil
				}
				return value("", &i, name)
			}
			switch name {
			case "--help":
				return opts, nil, errHelp
			case "--fixed-strings":
				opts.fixed = true
			case "--ignore-case":
				opts.ignoreCase = true
			case "--smart-case":
				opts.smartCase = true
			case "--invert-match":
				opts.invert = true
			case "--word-regexp":
				opts.word = true
			case "--line-regexp":
				opts.wholeLine = true
			case "--hidden":
				opts.hidden = true
			case "--no-ignore":
				opts.noIgnore = true
			case "--text":
				opts.text = true
			case "--search-zip":
				opts.searchZip = true
			case "--glob-case-insensitive":
				opts.globCaseInsensitive = true
			case "--stats":
				opts.stats = true
			case "--follow":
				opts.follow = true
			case "--line-number":
				opts.lineNumber, opts.lineNumberExplicit = true, true
			case "--no-line-number":
				opts.lineNumber, opts.lineNumberExplicit = false, true
			case "--column":
				opts.column = true
			case "--with-filename":
				opts.withFilename, opts.noFilename = true, false
			case "--no-filename":
				opts.noFilename, opts.withFilename = true, false
			case "--count":
				opts.count = true
			case "--count-matches":
				opts.countMatches = true
			case "--files":
				opts.files = true
			case "--files-with-matches":
				opts.filesWithMatches = true
			case "--files-without-match":
				opts.filesWithoutMatch = true
			case "--only-matching":
				opts.onlyMatching = true
			case "--quiet":
				opts.quiet = true
			case "--json":
				opts.json = true
			case "--no-heading":
				opts.noHeading = true
			case "--null":
				opts.nullFilename = true
			case "--null-data":
				opts.nullData = true
			case "--type-list":
				opts.typeList = true
			case "--glob", "-g":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				opts.globs = append(opts.globs, v)
			case "--type", "-t":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				opts.types = append(opts.types, v)
			case "--type-not", "-T":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				opts.excludedTypes = append(opts.excludedTypes, v)
			case "--type-add":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				name, glob, ok := strings.Cut(v, ":")
				if !ok || name == "" || glob == "" {
					return opts, nil, fmt.Errorf("invalid type definition %q", v)
				}
				opts.typeDefs[name] = append(opts.typeDefs[name], glob)
			case "--regexp", "-e":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				opts.patterns = append(opts.patterns, v)
			case "--file", "-f":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				var data []byte
				if v == "-" {
					data, err = io.ReadAll(ioEnv.Stdin)
					if err != nil {
						return opts, nil, fmt.Errorf("read patterns from standard input: %w", err)
					}
					opts.stdin = bytes.NewReader(nil)
				} else {
					data, err = os.ReadFile(ioEnv.ResolvePath(v))
				}
				if err != nil {
					return opts, nil, fmt.Errorf("read pattern file %q: %w", v, err)
				}
				for _, p := range splitPatterns(data) {
					opts.patterns = append(opts.patterns, p)
				}
			case "--max-count":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				opts.maxCount, err = number(v, "max-count")
				if err != nil {
					return opts, nil, err
				}
			case "--maxdepth", "--max-depth":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				opts.maxDepth, err = number(v, "maxdepth")
				if err != nil {
					return opts, nil, err
				}
			case "--max-filesize":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				opts.maxFileSize, err = parseSize(v)
				if err != nil {
					return opts, nil, err
				}
			case "--before-context", "--after-context", "--context":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				n, err := number(v, "context")
				if err != nil {
					return opts, nil, err
				}
				switch name {
				case "--before-context":
					opts.before = n
				case "--after-context":
					opts.after = n
				default:
					opts.before, opts.after = n, n
				}
			case "--color":
				if hasValue {
					opts.color = val
				} else {
					opts.color = "always"
				}
				if opts.color != "always" && opts.color != "never" && opts.color != "auto" {
					return opts, nil, fmt.Errorf("invalid color mode %q", opts.color)
				}
			case "--sort":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				if v != "path" && v != "none" {
					return opts, nil, fmt.Errorf("unsupported sort mode %q", v)
				}
				opts.sort = v
			case "--context-separator":
				v, err := getValue()
				if err != nil {
					return opts, nil, err
				}
				opts.contextSeparator = v
			default:
				return opts, nil, fmt.Errorf("unknown option %q", arg)
			}
			continue
		}
		if !endOptions && strings.HasPrefix(arg, "-") && arg != "-" {
			for j := 1; j < len(arg); j++ {
				flag, attached := arg[j], arg[j+1:]
				switch flag {
				case 'e', 'f', 'g', 't', 'T', 'm', 'A', 'B', 'C':
					v, err := value(attached, &i, "-"+string(flag))
					if err != nil {
						return opts, nil, err
					}
					j = len(arg)
					switch flag {
					case 'e':
						opts.patterns = append(opts.patterns, v)
					case 'f':
						var data []byte
						if v == "-" {
							data, err = io.ReadAll(ioEnv.Stdin)
							if err != nil {
								return opts, nil, fmt.Errorf("read patterns from standard input: %w", err)
							}
							opts.stdin = bytes.NewReader(nil)
						} else {
							data, err = os.ReadFile(ioEnv.ResolvePath(v))
						}
						if err != nil {
							return opts, nil, fmt.Errorf("read pattern file %q: %w", v, err)
						}
						opts.patterns = append(opts.patterns, splitPatterns(data)...)
					case 'g':
						opts.globs = append(opts.globs, v)
					case 't':
						opts.types = append(opts.types, v)
					case 'T':
						opts.excludedTypes = append(opts.excludedTypes, v)
					case 'm':
						opts.maxCount, err = number(v, "max-count")
					case 'A', 'B', 'C':
						var n int
						n, err = number(v, "context")
						if err == nil {
							switch flag {
							case 'A':
								opts.after = n
							case 'B':
								opts.before = n
							case 'C':
								opts.before, opts.after = n, n
							}
						}
					}
					if err != nil {
						return opts, nil, err
					}
				default:
					switch flag {
					case 'F':
						opts.fixed = true
					case 'i':
						opts.ignoreCase = true
					case 'S':
						opts.smartCase = true
					case 'v':
						opts.invert = true
					case 'w':
						opts.word = true
					case 'x':
						opts.wholeLine = true
					case 'n':
						opts.lineNumber, opts.lineNumberExplicit = true, true
					case 'N':
						opts.lineNumber, opts.lineNumberExplicit = false, true
					case 'H':
						opts.withFilename, opts.noFilename = true, false
					case 'h':
						opts.noFilename, opts.withFilename = true, false
					case 'a':
						opts.text = true
					case 'L':
						opts.follow = true
					case '0':
						opts.nullData = true
					case 'o':
						opts.onlyMatching = true
					case 'l':
						opts.filesWithMatches = true
					case 'q':
						opts.quiet = true
					case 's': // ripgrep's -s explicitly keeps case-sensitive matching.
					case 'u':
						if !opts.noIgnore {
							opts.noIgnore = true
						} else if !opts.hidden {
							opts.hidden = true
						} else {
							opts.text = true
						}
					case 'P':
						opts.perl = true
					case 'U':
						opts.multiline = true
					case 'z':
						opts.searchZip = true
					case 'c':
						opts.count = true
					default:
						return opts, nil, fmt.Errorf("unknown option -%c", flag)
					}
				}
			}
			continue
		}
		positional = append(positional, arg)
	}
	if len(opts.patterns) == 0 && !opts.files && !opts.typeList {
		if len(positional) == 0 {
			return opts, nil, fmt.Errorf("missing pattern")
		}
		opts.patterns = append(opts.patterns, positional[0])
		positional = positional[1:]
	}
	if opts.smartCase {
		hasUpper := strings.ContainsFunc(strings.Join(opts.patterns, " "), func(r rune) bool { return r >= 'A' && r <= 'Z' })
		opts.ignoreCase = !hasUpper
	}
	for _, name := range append(append([]string(nil), opts.types...), opts.excludedTypes...) {
		if _, ok := opts.typeDefs[name]; !ok {
			return opts, nil, fmt.Errorf("unknown file type %q", name)
		}
	}
	if opts.typeList && opts.json {
		return opts, nil, fmt.Errorf("--json cannot be combined with --type-list")
	}
	if !opts.files && !opts.typeList && len(opts.patterns) == 0 {
		return opts, nil, fmt.Errorf("missing pattern")
	}
	paths = append(paths, positional...)
	if len(paths) == 0 {
		if opts.stdin == nil && !ioEnv.StdinIsTerminal {
			paths = []string{"-"}
		} else {
			paths = []string{"."}
		}
	}
	return opts, paths, nil
}

func parseSize(s string) (int64, error) {
	multiplier := int64(1)
	if len(s) > 0 {
		switch strings.ToLower(s[len(s)-1:]) {
		case "k":
			multiplier, s = 1<<10, s[:len(s)-1]
		case "m":
			multiplier, s = 1<<20, s[:len(s)-1]
		case "g":
			multiplier, s = 1<<30, s[:len(s)-1]
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid max-filesize")
	}
	return n * multiplier, nil
}

func (c *command) run(paths []string) error {
	c.multiplePaths = len(paths) > 1
	if len(paths) == 1 && paths[0] == "-" {
		return c.searchReader(c.ioEnv.Stdin, "(standard input)", false)
	}
	for _, path := range paths {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		if path == "-" {
			if err := c.searchReader(c.ioEnv.Stdin, "(standard input)", false); err != nil {
				return err
			}
			continue
		}
		resolved := c.ioEnv.ResolvePath(path)
		display := filepath.ToSlash(path)
		var inherited []ignoreRule
		var err error
		if !c.opts.noIgnore {
			inherited, err = c.globalIgnoreRules(resolved)
			if err != nil {
				c.hadError = true
				fmt.Fprintf(c.errOut, "rg: %s: %v\n", display, err)
				continue
			}
		}
		if err := c.searchPath(resolved, display, inherited, 0, false); err != nil {
			c.hadError = true
			fmt.Fprintf(c.errOut, "rg: %s: %v\n", display, err)
		}
	}
	return nil
}

func (c *command) searchPath(path, display string, inherited []ignoreRule, depth int, fromWalk bool) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if fromWalk && !c.opts.follow {
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
		if fromWalk && c.opts.maxDepth >= 0 && depth >= c.opts.maxDepth {
			return nil
		}
		if fromWalk && hiddenPath(display) && !c.opts.hidden {
			return nil
		}
		if fromWalk && !c.opts.noIgnore && ignored(display, true, inherited) {
			return nil
		}
		if c.wasSeen(path) {
			return nil
		}
		rules := append([]ignoreRule(nil), inherited...)
		if !c.opts.noIgnore {
			localRules, err := loadIgnoreRules(path, display)
			if err != nil {
				return err
			}
			rules = append(rules, localRules...)
		}
		var entries []os.DirEntry
		if c.opts.sort == "path" {
			entries, err = os.ReadDir(path)
		} else {
			dir, openErr := os.Open(path)
			if openErr != nil {
				return openErr
			}
			entries, err = dir.ReadDir(-1)
			closeErr := dir.Close()
			if err == nil {
				err = closeErr
			}
		}
		if err != nil {
			return err
		}
		if c.opts.sort == "path" {
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		}
		for _, entry := range entries {
			if err := c.ctx.Err(); err != nil {
				return err
			}
			childDisplay := pathpkg.Join(filepath.ToSlash(display), entry.Name())
			if !c.opts.hidden && hiddenPath(entry.Name()) {
				continue
			}
			if !c.opts.noIgnore && ignored(childDisplay, entry.IsDir(), rules) {
				continue
			}
			if !entry.IsDir() && !c.pathSelected(childDisplay, false) {
				continue
			}
			if err := c.searchPath(filepath.Join(path, entry.Name()), childDisplay, rules, depth+1, true); err != nil {
				c.hadError = true
				fmt.Fprintf(c.errOut, "rg: %s: %v\n", childDisplay, err)
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if !c.pathSelected(display, false) {
		return nil
	}
	if c.opts.maxFileSize > 0 && info.Size() > c.opts.maxFileSize {
		return nil
	}
	if c.opts.files {
		_, err := io.WriteString(c.out, display+outputDelimiter(c.opts.nullFilename))
		c.found = true
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	display = filepath.ToSlash(display)
	if c.opts.searchZip {
		reader, closeReader, err := decompressedReader(file, display)
		if err != nil {
			return err
		}
		if closeReader != nil {
			defer closeReader.Close()
		}
		return c.searchReader(reader, display, c.filenameByDefault(fromWalk))
	}
	return c.searchReader(file, display, c.filenameByDefault(fromWalk))
}

func (c *command) filenameByDefault(fromWalk bool) bool {
	return c.opts.withFilename || c.multiplePaths || fromWalk || c.ioEnv.StdoutIsTerminal
}

func (c *command) wasSeen(path string) bool {
	if c.seen == nil {
		c.seen = make(map[string]struct{})
	}
	key, err := filepath.Abs(path)
	if err == nil {
		path = key
	}
	if _, ok := c.seen[path]; ok {
		return true
	}
	c.seen[path] = struct{}{}
	return false
}

func (c *command) searchReader(reader io.Reader, name string, named bool) error {
	name = filepath.ToSlash(name)
	if c.opts.multiline {
		return c.searchMultiline(reader, name, named)
	}
	fileStarted := time.Now()
	printedBefore := c.jsonBytesPrinted
	var offset int64
	var selected, totalMatches int
	matchedFile := false
	c.stats.filesSearched++
	defer func() {
		c.stats.bytesSearched += offset
		c.stats.matchedLines += int64(selected)
		c.stats.matches += int64(totalMatches)
		if matchedFile {
			c.stats.filesWithMatches++
		}
	}()
	delim := byte('\n')
	if c.opts.nullData {
		delim = 0
	}
	if c.opts.json && !c.opts.quiet {
		if err := c.jsonEvent("begin", map[string]any{"path": jsonPath(name)}); err != nil {
			return err
		}
	}
	if c.opts.maxCount == 0 {
		if c.opts.count || c.opts.countMatches {
			value := 0
			if named && c.showFilename(name) {
				sep := ":"
				if c.opts.nullFilename {
					sep = "\x00"
				}
				if _, err := fmt.Fprintf(c.out, "%s%s%d\n", name, sep, value); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintln(c.out, value); err != nil {
				return err
			}
		}
		if c.opts.filesWithoutMatch {
			_, err := io.WriteString(c.out, name+outputDelimiter(c.opts.nullFilename))
			c.found = true
			return err
		}
		if c.opts.json && !c.opts.quiet {
			return c.jsonEnd(name, nil, fileStarted, printedBefore, 0, 0, 0)
		}
		return nil
	}
	br := bufio.NewReader(reader)
	var before []line
	lineNumber := 0
	lastOut := 0
	afterLeft := 0
	binaryFile := false
	for {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		data, err := br.ReadBytes(delim)
		if len(data) == 0 && err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		lineNumber++
		lineOffset := offset
		offset += int64(len(data))
		terminated := false
		if len(data) > 0 && data[len(data)-1] == delim {
			data = data[:len(data)-1]
			terminated = true
		}
		item := line{data: append([]byte(nil), data...), number: lineNumber, offset: lineOffset, terminated: terminated}
		if c.opts.files {
			if _, e := fmt.Fprintln(c.out, name); e != nil {
				return e
			}
			return nil
		}
		if !c.opts.text && bytes.IndexByte(data, 0) >= 0 {
			binaryFile = true
			break
		}
		var matches [][]int
		if c.matcher != nil {
			matches = c.matcher.FindAllIndex(data, 0)
		}
		isMatch := len(matches) > 0
		if c.opts.invert {
			isMatch = !isMatch
		}
		if isMatch {
			selected++
			totalMatches += len(matches)
			matchedFile = true
			c.found = true
			if c.opts.quiet {
				return errStop
			}
			if c.opts.filesWithMatches {
				_, e := io.WriteString(c.out, name+outputDelimiter(c.opts.nullFilename))
				c.found = true
				return e
			}
			if c.opts.count || c.opts.countMatches {
				if c.opts.maxCount >= 0 && selected >= c.opts.maxCount {
					break
				}
				continue
			}
			for _, prev := range before {
				if prev.number > lastOut {
					if err := c.emit(name, prev, false, nil, named, &lastOut); err != nil {
						return err
					}
				}
			}
			if !c.opts.json && lastOut != 0 && item.number > lastOut+1 {
				if _, err := io.WriteString(c.out, c.opts.contextSeparator); err != nil {
					return err
				}
				if err := c.out.WriteByte(delimiter(c.opts.nullData)); err != nil {
					return err
				}
			}
			if err := c.emit(name, item, true, matches, named, &lastOut); err != nil {
				return err
			}
			afterLeft = c.opts.after
			before = nil
		} else if afterLeft > 0 {
			if err := c.emit(name, item, false, nil, named, &lastOut); err != nil {
				return err
			}
			afterLeft--
		} else if c.opts.before > 0 {
			before = append(before, item)
			if len(before) > c.opts.before {
				before = before[len(before)-c.opts.before:]
			}
		}
		if c.opts.maxCount >= 0 && selected >= c.opts.maxCount {
			break
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	if c.opts.filesWithoutMatch && selected == 0 && !binaryFile {
		_, err := io.WriteString(c.out, name+outputDelimiter(c.opts.nullFilename))
		c.found = true
		return err
	}
	if c.opts.count || c.opts.countMatches {
		value := selected
		if c.opts.countMatches {
			value = totalMatches
		}
		if named && c.showFilename(name) {
			sep := ":"
			if c.opts.nullFilename {
				sep = "\x00"
			}
			if _, err := fmt.Fprintf(c.out, "%s%s%d\n", name, sep, value); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(c.out, "%d\n", value); err != nil {
			return err
		}
	}
	if c.opts.json && !c.opts.quiet {
		return c.jsonEnd(name, nil, fileStarted, printedBefore, totalMatches, selected, offset)
	}
	return nil
}

func (c *command) searchMultiline(reader io.Reader, name string, named bool) error {
	fileStarted := time.Now()
	printedBefore := c.jsonBytesPrinted
	c.stats.filesSearched++
	if c.opts.json && !c.opts.quiet {
		if err := c.jsonEvent("begin", map[string]any{"path": jsonPath(name)}); err != nil {
			return err
		}
	}
	if c.opts.maxCount == 0 {
		if c.opts.count || c.opts.countMatches {
			if named && c.showFilename(name) {
				sep := ":"
				if c.opts.nullFilename {
					sep = "\x00"
				}
				if _, err := fmt.Fprintf(c.out, "%s%s0\n", name, sep); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintln(c.out, 0); err != nil {
				return err
			}
		}
		if c.opts.filesWithoutMatch {
			c.found = true
			return c.writeFileName(name)
		}
		if c.opts.json && !c.opts.quiet {
			return c.jsonEnd(name, nil, fileStarted, printedBefore, 0, 0, 0)
		}
		return nil
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	c.stats.bytesSearched += int64(len(data))
	if !c.opts.text && bytes.IndexByte(data, 0) >= 0 {
		if c.opts.json && !c.opts.quiet {
			return c.jsonEnd(name, bytes.IndexByte(data, 0), fileStarted, printedBefore, 0, 0, int64(len(data)))
		}
		return nil
	}

	delim := byte('\n')
	if c.opts.nullData {
		delim = 0
	}
	lines := splitLines(data, delim)
	allMatches := c.matcher.FindAllIndex(data, 0)
	lineMatches := make([][][]int, len(lines))
	lineMatchCounts := make([]int, len(lines))
	lineSelected := make([]bool, len(lines))
	for _, span := range allMatches {
		for i, item := range lines {
			recordStart := int(item.offset)
			contentEnd := recordStart + len(item.data)
			recordEnd := contentEnd
			if item.terminated {
				recordEnd++
			}
			overlaps := span[0] < recordEnd && span[1] > recordStart
			if span[0] == span[1] && span[0] >= recordStart && span[0] <= recordEnd {
				overlaps = true
			}
			if !overlaps {
				continue
			}
			lineSelected[i] = true
			if span[0] >= recordStart && span[0] <= contentEnd {
				lineMatchCounts[i]++
			}
			start := max(span[0], recordStart)
			end := min(span[1], contentEnd)
			if start < end {
				lineMatches[i] = append(lineMatches[i], []int{start - recordStart, end - recordStart})
			}
		}
	}

	selected, totalMatches := 0, 0
	matchedFile := false
	lastOut, afterLeft := 0, 0
	var before []line
	for i, item := range lines {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		matchLine := lineSelected[i]
		isSelected := matchLine
		if c.opts.invert {
			isSelected = !matchLine
		}
		if isSelected {
			selected++
			matchedFile = true
			c.found = true
			totalMatches += lineMatchCounts[i]
			if c.opts.quiet {
				c.stats.matches += int64(totalMatches)
				c.stats.matchedLines += int64(selected)
				c.stats.filesWithMatches++
				return errStop
			}
			if c.opts.filesWithMatches {
				c.stats.matches += int64(totalMatches)
				c.stats.matchedLines += int64(selected)
				c.stats.filesWithMatches++
				return c.writeFileName(name)
			}
			if !c.opts.count && !c.opts.countMatches {
				for _, old := range before {
					if old.number > lastOut {
						if err := c.emit(name, old, false, nil, named, &lastOut); err != nil {
							return err
						}
					}
				}
				if !c.opts.json && lastOut != 0 && item.number > lastOut+1 {
					if _, err := io.WriteString(c.out, c.opts.contextSeparator); err != nil {
						return err
					}
					if err := c.out.WriteByte(delimiter(c.opts.nullData)); err != nil {
						return err
					}
				}
				spans := lineMatches[i]
				if c.opts.invert {
					spans = nil
				}
				if err := c.emit(name, item, true, spans, named, &lastOut); err != nil {
					return err
				}
				afterLeft = c.opts.after
				before = nil
			}
		} else if afterLeft > 0 && !c.opts.count && !c.opts.countMatches {
			if err := c.emit(name, item, false, nil, named, &lastOut); err != nil {
				return err
			}
			afterLeft--
		} else if c.opts.before > 0 && !c.opts.count && !c.opts.countMatches {
			before = append(before, item)
			if len(before) > c.opts.before {
				before = before[len(before)-c.opts.before:]
			}
		}
		if c.opts.maxCount > 0 && selected >= c.opts.maxCount {
			break
		}
	}
	if selected > 0 {
		c.stats.matches += int64(totalMatches)
		c.stats.matchedLines += int64(selected)
		c.stats.filesWithMatches++
	}
	if c.opts.count || c.opts.countMatches {
		value := selected
		if c.opts.countMatches {
			value = totalMatches
		}
		if named && c.showFilename(name) {
			sep := ":"
			if c.opts.nullFilename {
				sep = "\x00"
			}
			if _, err := fmt.Fprintf(c.out, "%s%s%d\n", name, sep, value); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(c.out, "%d\n", value); err != nil {
			return err
		}
	}
	if c.opts.filesWithoutMatch && !matchedFile {
		c.found = true
		return c.writeFileName(name)
	}
	if c.opts.json && !c.opts.quiet {
		return c.jsonEnd(name, nil, fileStarted, printedBefore, totalMatches, selected, int64(len(data)))
	}
	return nil
}

func splitLines(data []byte, delim byte) []line {
	var lines []line
	for offset := 0; offset < len(data); {
		end := bytes.IndexByte(data[offset:], delim)
		if end < 0 {
			lines = append(lines, line{
				data: append([]byte(nil), data[offset:]...), number: len(lines) + 1,
				offset: int64(offset),
			})
			break
		}
		recordEnd := offset + end
		lines = append(lines, line{
			data: append([]byte(nil), data[offset:recordEnd]...), number: len(lines) + 1,
			offset: int64(offset), terminated: true,
		})
		offset = recordEnd + 1
	}
	return lines
}

type line struct {
	data       []byte
	number     int
	offset     int64
	terminated bool
}

func (c *command) emit(name string, item line, matched bool, matches [][]int, named bool, lastOut *int) error {
	if item.number <= *lastOut {
		return nil
	}
	if c.opts.json && !c.opts.quiet {
		if err := c.jsonLine(name, item, matches, matched); err != nil {
			return err
		}
		*lastOut = item.number
		return nil
	}
	sep := ":"
	if !matched {
		sep = "-"
	}
	prefix := ""
	if named && c.showFilename(name) {
		if c.opts.nullFilename {
			prefix += name + "\x00"
		} else {
			prefix += name + sep
		}
	}
	showLineNumber := c.opts.lineNumber ||
		(!c.opts.lineNumberExplicit && c.ioEnv.StdoutIsTerminal)
	if showLineNumber {
		prefix += strconv.Itoa(item.number) + sep
	}
	if c.opts.column && matched && len(matches) > 0 {
		prefix += strconv.Itoa(matches[0][0]+1) + sep
	}
	if c.opts.onlyMatching && matched {
		for _, span := range matches {
			part := item.data[span[0]:span[1]]
			if _, err := io.WriteString(c.out, prefix); err != nil {
				return err
			}
			if c.color {
				_, _ = io.WriteString(c.out, "\x1b[1;31m")
			}
			if _, err := c.out.Write(part); err != nil {
				return err
			}
			if c.color {
				_, _ = io.WriteString(c.out, "\x1b[0m")
			}
			if err := c.out.WriteByte(delimiter(c.opts.nullData)); err != nil {
				return err
			}
		}
	} else {
		if _, err := io.WriteString(c.out, prefix); err != nil {
			return err
		}
		if c.color && matched {
			start := 0
			for _, span := range matches {
				if span[0] < start {
					continue
				}
				if _, err := c.out.Write(item.data[start:span[0]]); err != nil {
					return err
				}
				_, _ = io.WriteString(c.out, "\x1b[1;31m")
				if _, err := c.out.Write(item.data[span[0]:span[1]]); err != nil {
					return err
				}
				_, _ = io.WriteString(c.out, "\x1b[0m")
				start = span[1]
			}
			if _, err := c.out.Write(item.data[start:]); err != nil {
				return err
			}
		} else if _, err := c.out.Write(item.data); err != nil {
			return err
		}
		if err := c.out.WriteByte(delimiter(c.opts.nullData)); err != nil {
			return err
		}
	}
	*lastOut = item.number
	return nil
}

func (c *command) showFilename(name string) bool {
	if c.opts.noFilename {
		return false
	}
	return c.opts.withFilename || name != "(standard input)"
}

func (c *command) writeFileName(name string) error {
	if _, err := io.WriteString(c.out, filepath.ToSlash(name)); err != nil {
		return err
	}
	return c.out.WriteByte(byte(outputDelimiter(c.opts.nullFilename)[0]))
}

func delimiter(null bool) byte {
	if null {
		return 0
	}
	return '\n'
}

func outputDelimiter(null bool) string {
	if null {
		return "\x00"
	}
	return "\n"
}
func hiddenPath(p string) bool {
	for _, part := range strings.Split(filepath.ToSlash(p), "/") {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}

func (c *command) pathSelected(path string, dir bool) bool {
	path = filepath.ToSlash(path)
	for _, name := range c.opts.types {
		globs, ok := c.opts.typeDefs[name]
		if !ok {
			return false
		}
		matched := false
		for _, glob := range globs {
			if globMatch(glob, filepath.Base(path)) {
				matched = true
			}
		}
		if !matched {
			return false
		}
	}
	for _, name := range c.opts.excludedTypes {
		globs, ok := c.opts.typeDefs[name]
		if !ok {
			return false
		}
		for _, glob := range globs {
			if globMatch(glob, filepath.Base(path)) {
				return false
			}
		}
	}
	if len(c.opts.globs) == 0 {
		return true
	}
	hasPositive := false
	for _, glob := range c.opts.globs {
		if !strings.HasPrefix(glob, "!") {
			hasPositive = true
			break
		}
	}
	allow := !hasPositive
	for _, glob := range c.opts.globs {
		negate := strings.HasPrefix(glob, "!")
		if negate {
			glob = strings.TrimPrefix(glob, "!")
		}
		if globMatchCase(glob, path, c.opts.globCaseInsensitive) || globMatchCase(glob, filepath.Base(path), c.opts.globCaseInsensitive) {
			allow = !negate
		}
	}
	_ = dir
	return allow
}

func globMatch(pattern, name string) bool {
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)
	if strings.Contains(pattern, "**") {
		re := regexp.QuoteMeta(pattern)
		re = strings.ReplaceAll(re, `\*\*/`, `(?:.*/)?`)
		re = strings.ReplaceAll(re, `\*\*`, `.*`)
		re = strings.ReplaceAll(re, `\*`, `[^/]*`)
		re = strings.ReplaceAll(re, `\?`, `[^/]`)
		ok, _ := regexp.MatchString("^"+re+"$", name)
		return ok
	}
	if ok, _ := filepath.Match(pattern, name); ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		_, base := filepath.Split(name)
		ok, _ := filepath.Match(pattern, base)
		return ok
	}
	return false
}

func globMatchCase(pattern, name string, ignoreCase bool) bool {
	if !ignoreCase {
		return globMatch(pattern, name)
	}
	return globMatch(strings.ToLower(pattern), strings.ToLower(name))
}

type closeFunc func() error

func (f closeFunc) Close() error {
	return f()
}

func decompressedReader(reader io.Reader, name string) (io.Reader, io.Closer, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".gz", ".gzip":
		decoded, err := gzip.NewReader(reader)
		if err != nil {
			return nil, nil, fmt.Errorf("open gzip stream: %w", err)
		}
		return decoded, decoded, nil
	case ".bz2", ".bzip2":
		return bzip2.NewReader(reader), nil, nil
	case ".xz":
		decoded, err := xz.NewReader(reader)
		if err != nil {
			return nil, nil, fmt.Errorf("open xz stream: %w", err)
		}
		return decoded, nil, nil
	case ".lz4":
		return lz4.NewReader(reader), nil, nil
	case ".lzma":
		decoded, err := lzma.NewReader(reader)
		if err != nil {
			return nil, nil, fmt.Errorf("open lzma stream: %w", err)
		}
		return decoded, nil, nil
	case ".zst", ".zstd":
		decoded, err := zstd.NewReader(reader)
		if err != nil {
			return nil, nil, fmt.Errorf("open zstd stream: %w", err)
		}
		return decoded, closeFunc(func() error {
			decoded.Close()
			return nil
		}), nil
	default:
		return reader, nil, nil
	}
}

func loadIgnoreRules(dir, display string) ([]ignoreRule, error) {
	var out []ignoreRule
	// Lower-precedence files are loaded first; later local rules override them.
	for _, file := range []string{".gitignore", ".ignore", ".rgignore"} {
		filename := filepath.Join(dir, file)
		data, found, err := readOptionalFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read ignore file %q: %w", filename, err)
		}
		if found {
			out = append(out, parseIgnoreRules(data, filepath.ToSlash(display))...)
		}
	}
	return out, nil
}

func parseIgnoreRules(data []byte, base string) []ignoreRule {
	var out []ignoreRule
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := ignoreRule{base: filepath.ToSlash(base), pattern: filepath.ToSlash(line)}
		if strings.HasPrefix(rule.pattern, "!") {
			rule.negate, rule.pattern = true, strings.TrimPrefix(rule.pattern, "!")
		}
		if strings.HasPrefix(rule.pattern, "/") {
			rule.anchored, rule.pattern = true, strings.TrimPrefix(rule.pattern, "/")
		}
		if strings.HasSuffix(rule.pattern, "/") {
			rule.dirOnly, rule.pattern = true, strings.TrimSuffix(rule.pattern, "/")
		}
		out = append(out, rule)
	}
	return out
}

func (c *command) globalIgnoreRules(start string) ([]ignoreRule, error) {
	var out []ignoreRule
	home := c.ioEnv.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	globalConfig := c.ioEnv.Getenv("GIT_CONFIG_GLOBAL")
	var configPaths []string
	if globalConfig != "" {
		configPaths = append(configPaths, globalConfig)
	} else if home != "" {
		configPaths = append(configPaths,
			filepath.Join(home, ".gitconfig"),
			filepath.Join(home, ".config", "git", "config"),
		)
	}
	excludesFile := ""
	for _, configPath := range configPaths {
		data, found, err := readOptionalFile(c.ioEnv.ResolvePath(configPath))
		if err != nil {
			return nil, fmt.Errorf("read Git config %q: %w", configPath, err)
		}
		if !found {
			continue
		}
		excludesFile = configuredExcludesFile(string(data))
		if excludesFile != "" {
			if strings.HasPrefix(excludesFile, "~/") && home != "" {
				excludesFile = filepath.Join(home, filepath.FromSlash(excludesFile[2:]))
			} else if !filepath.IsAbs(excludesFile) {
				excludesFile = filepath.Join(filepath.Dir(configPath), excludesFile)
			}
		}
		if excludesFile != "" {
			break
		}
	}
	if excludesFile == "" && home != "" {
		excludesFile = filepath.Join(home, ".config", "git", "ignore")
	}
	if excludesFile != "" {
		data, found, err := readOptionalFile(c.ioEnv.ResolvePath(excludesFile))
		if err != nil {
			return nil, fmt.Errorf("read global Git excludes %q: %w", excludesFile, err)
		}
		if found {
			out = append(out, parseIgnoreRules(data, ".")...)
		}
	}
	gitDir, err := findGitDir(start)
	if err != nil {
		return nil, err
	}
	if gitDir != "" {
		filename := filepath.Join(gitDir, "info", "exclude")
		data, found, err := readOptionalFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read Git info exclude %q: %w", filename, err)
		}
		if found {
			out = append(out, parseIgnoreRules(data, ".")...)
		}
	}
	return out, nil
}

func readOptionalFile(filename string) ([]byte, bool, error) {
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func configuredExcludesFile(config string) string {
	inCore := false
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inCore = strings.EqualFold(strings.TrimSpace(line[1:len(line)-1]), "core")
			continue
		}
		if !inCore {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "excludesfile") {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"`)
	}
	return ""
}

func findGitDir(start string) (string, error) {
	dir := start
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		candidate := filepath.Join(dir, ".git")
		info, err := os.Stat(candidate)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect Git directory %q: %w", candidate, err)
		}
		if err == nil {
			if info.IsDir() {
				return candidate, nil
			}
			data, err := os.ReadFile(candidate)
			if err == nil {
				line := strings.TrimSpace(string(data))
				if strings.HasPrefix(strings.ToLower(line), "gitdir:") {
					target := strings.TrimSpace(line[len("gitdir:"):])
					if !filepath.IsAbs(target) {
						target = filepath.Join(dir, target)
					}
					return filepath.Clean(target), nil
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("read Git directory marker %q: %w", candidate, err)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func ignored(path string, isDir bool, rules []ignoreRule) bool {
	path = filepath.ToSlash(path)
	ignored := false
	for _, rule := range rules {
		rel := path
		if rule.base != "." {
			prefix := rule.base + "/"
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			rel = strings.TrimPrefix(path, prefix)
		}
		if rule.dirOnly && !isDir && !strings.Contains(rel, rule.pattern+"/") {
			continue
		}
		matched := globMatch(rule.pattern, rel)
		if !matched && !rule.anchored && !strings.Contains(rule.pattern, "/") {
			for _, part := range strings.Split(rel, "/") {
				if globMatch(rule.pattern, part) {
					matched = true
					break
				}
			}
		}
		if matched {
			ignored = !rule.negate
		}
	}
	return ignored
}

var fileTypes = map[string][]string{
	"c": {"*.c", "*.h"}, "cpp": {"*.cc", "*.cpp", "*.cxx", "*.hpp", "*.hh"},
	"css": {"*.css"}, "go": {"*.go"}, "html": {"*.html", "*.htm"},
	"java": {"*.java"}, "js": {"*.js", "*.jsx", "*.mjs", "*.cjs"},
	"json": {"*.json"}, "md": {"*.md", "*.markdown"}, "py": {"*.py"},
	"rs": {"*.rs"}, "sh": {"*.sh", "*.bash"}, "toml": {"*.toml"},
	"txt": {"*.txt"}, "yaml": {"*.yaml", "*.yml"},
}

func cloneTypes() map[string][]string {
	out := make(map[string][]string, len(fileTypes))
	for name, patterns := range fileTypes {
		out[name] = append([]string(nil), patterns...)
	}
	return out
}

func splitPatterns(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := bytes.Split(data, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 && data[len(data)-1] == '\n' {
		lines = lines[:len(lines)-1]
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, string(line))
	}
	return out
}

func jsonPath(path string) map[string]string {
	path = filepath.ToSlash(path)
	if utf8.ValidString(path) {
		return map[string]string{"text": path}
	}
	return map[string]string{"bytes": base64.StdEncoding.EncodeToString([]byte(path))}
}

func (c *command) jsonLine(name string, item line, spans [][]int, matched bool) error {
	lineBytes := append([]byte(nil), item.data...)
	if item.terminated {
		lineBytes = append(lineBytes, delimiter(c.opts.nullData))
	}
	lineValue := map[string]string{"text": string(lineBytes)}
	if !utf8.Valid(item.data) {
		lineValue = map[string]string{"bytes": base64.StdEncoding.EncodeToString(lineBytes)}
	}
	lineNumber := item.number
	event := map[string]any{
		"type": map[bool]string{true: "match", false: "context"}[matched],
		"data": map[string]any{
			"path":            jsonPath(name),
			"lines":           lineValue,
			"line_number":     lineNumber,
			"absolute_offset": item.offset,
			"submatches":      []any{},
		},
	}
	sub := make([]map[string]any, 0, len(spans))
	for _, span := range spans {
		matchValue := map[string]string{"text": string(item.data[span[0]:span[1]])}
		if !utf8.Valid(item.data[span[0]:span[1]]) {
			matchValue = map[string]string{"bytes": base64.StdEncoding.EncodeToString(item.data[span[0]:span[1]])}
		}
		sub = append(sub, map[string]any{"match": matchValue, "start": span[0], "end": span[1]})
	}
	event["data"].(map[string]any)["submatches"] = sub
	return c.jsonEvent(event["type"].(string), event["data"].(map[string]any))
}

func (c *command) jsonEvent(eventType string, data map[string]any) error {
	encoded, err := json.Marshal(map[string]any{"type": eventType, "data": data})
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	n, err := c.out.Write(encoded)
	if eventType == "begin" || eventType == "match" {
		c.jsonBytesPrinted += int64(n)
	}
	return err
}

func (c *command) bytesPrinted() int64 {
	return c.stdoutCounter.written + int64(c.out.Buffered())
}

func elapsedJSON(d time.Duration) map[string]any {
	if d < 0 {
		d = 0
	}
	return map[string]any{
		"secs":  int64(d / time.Second),
		"nanos": int64(d % time.Second),
		"human": fmt.Sprintf("%.6fs", d.Seconds()),
	}
}

func (c *command) jsonEnd(name string, binaryOffset any, started time.Time, printedBefore int64, matches, matchedLines int, bytesSearched int64) error {
	stats := map[string]any{
		"elapsed":             elapsedJSON(time.Since(started)),
		"searches":            1,
		"searches_with_match": boolInt(matchedLines > 0),
		"bytes_searched":      bytesSearched,
		"bytes_printed":       c.jsonBytesPrinted - printedBefore,
		"matched_lines":       matchedLines,
		"matches":             matches,
	}
	return c.jsonEvent("end", map[string]any{
		"path": jsonPath(name), "binary_offset": binaryOffset, "stats": stats,
	})
}

func (c *command) writeJSONSummary() error {
	if !c.opts.json || c.opts.quiet {
		return nil
	}
	c.stats.bytesPrinted = c.jsonBytesPrinted
	stats := map[string]any{
		"elapsed":             elapsedJSON(c.searchElapsed),
		"searches":            c.stats.filesSearched,
		"searches_with_match": c.stats.filesWithMatches,
		"bytes_searched":      c.stats.bytesSearched,
		"bytes_printed":       c.stats.bytesPrinted,
		"matched_lines":       c.stats.matchedLines,
		"matches":             c.stats.matches,
	}
	return c.jsonEvent("summary", map[string]any{
		"elapsed_total": elapsedJSON(time.Since(c.started)),
		"stats":         stats,
	})
}

func (c *command) writeStats() error {
	if !c.opts.stats || c.opts.json {
		return nil
	}
	searchElapsed := c.searchElapsed.Seconds()
	totalElapsed := time.Since(c.started).Seconds()
	_, err := fmt.Fprintf(c.ioEnv.Stdout,
		"\n%d matches\n%d matched lines\n%d files contained matches\n%d files searched\n%d bytes printed\n%d bytes searched\n%.6f seconds spent searching\n%.6f seconds total\n",
		c.stats.matches, c.stats.matchedLines, c.stats.filesWithMatches, c.stats.filesSearched,
		c.stdoutCounter.written, c.stats.bytesSearched, searchElapsed, totalElapsed)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
