// Copyright 2013-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// ls prints directory contents.
package lscmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/ls"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type cmd struct {
	w, errW        io.Writer
	all            bool
	almostAll      bool
	backup         bool
	human          bool
	directory      bool
	long           bool
	quoted         bool
	quoteStyle     string
	quiet          bool
	escape         bool
	rawNames       bool
	recurse        bool
	classify       bool
	fileType       bool
	dirsOnly       bool
	sizeSort       bool
	timeSort       bool
	extension      bool
	version        bool
	reverse        bool
	noSort         bool
	groupDirs      bool
	showInode      bool
	showBlocks     bool
	blockSizeK     bool
	comma          bool
	columns        bool
	across         bool
	single         bool
	followLinks    bool
	followCmdLinks bool
	omitUser       bool
	omitGroup      bool
	numericOwners  bool
	colorMode      string
	iconMode       string
	linkMode       string
	sortMode       string
	indicatorStyle string
	formatMode     string
	timeStyle      string
	fullTime       bool
	ignore         []string
	hide           []string
	timeMode       string
	timeModeAtime  bool
	timeModeCtime  bool
	force          bool
	noColor        bool
	terminal       bool
	width          int
	env            func(string) string
	resolvePath    func(string) string
}

type file struct {
	path   string
	name   string
	info   os.FileInfo
	lsInfo ls.FileInfo
	err    error
	isDir  bool
}

type literalNameStringer struct{}

func (literalNameStringer) FileString(fi ls.FileInfo) string { return fi.Name }

func indicator(fi ls.FileInfo) string {
	switch {
	case fi.Mode.IsRegular() && fi.Mode&0o111 != 0:
		return "*"
	case fi.Mode&os.ModeDir != 0:
		return "/"
	case fi.Mode&os.ModeSymlink != 0:
		return "@"
	case fi.Mode&os.ModeSocket != 0:
		return "="
	case fi.Mode&os.ModeNamedPipe != 0:
		return "|"
	case fi.Mode&os.ModeDevice != 0 && fi.Mode&os.ModeCharDevice != 0:
		return ""
	case fi.Mode&os.ModeDevice != 0:
		return ""
	default:
		return ""
	}
}

func (c cmd) selectedTime(info ls.FileInfo) int64 {
	t := info.MTime
	switch c.timeMode {
	case "atime":
		if v := reflectedTime(info, "ATime"); !v.IsZero() {
			t = v
		}
	case "ctime", "status":
		if v := reflectedTime(info, "CTime"); !v.IsZero() {
			t = v
		}
	}
	return t.UnixNano()
}

func reflectedInt(value any, name string) int64 {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		return 0
	}
	switch f.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return f.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(f.Uint())
	default:
		return 0
	}
}

func hasReflectedField(value any, name string) bool {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	return v.Kind() == reflect.Struct && v.FieldByName(name).IsValid()
}

func reflectedTime(value any, name string) (t time.Time) {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return time.Time{}
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		f := v.FieldByName(name)
		if f.IsValid() && f.CanInterface() {
			t, _ = f.Interface().(time.Time)
		}
	}
	return t
}

func timeLayout(style string, full bool) string {
	if full || style == "full-iso" {
		return "2006-01-02 15:04:05.000000000 -0700"
	}
	switch style {
	case "iso":
		return "2006-01-02"
	case "long-iso":
		return "2006-01-02 15:04"
	default:
		return "Jan _2 15:04"
	}
}

func (c cmd) resolve(path string) string {
	if c.resolvePath != nil {
		return c.resolvePath(path)
	}
	return path
}

func (c cmd) makeFile(path, name string, info os.FileInfo) file {
	if c.followLinks {
		followed, err := os.Stat(path)
		if err != nil {
			return file{path: path, name: name, err: err}
		}
		info = followed
	}
	fi := ls.FromOSFileInfo(path, info)
	fi.Name = name
	isDir := info.IsDir()
	return file{path: path, name: name, info: info, lsInfo: fi, isDir: isDir}
}

func (c cmd) shouldShow(name string) bool {
	if name == "." || name == ".." {
		return c.all
	}
	if strings.HasPrefix(name, ".") && !c.all && !c.almostAll {
		return false
	}
	if c.backup && strings.HasSuffix(name, "~") {
		return false
	}
	if !c.all && !c.almostAll {
		for _, pattern := range c.hide {
			if ok, _ := filepath.Match(pattern, name); ok {
				return false
			}
		}
	}
	for _, pattern := range c.ignore {
		if ok, _ := filepath.Match(pattern, name); ok {
			return false
		}
	}
	return true
}

func (c cmd) sortFiles(files []file) {
	if c.noSort {
		return
	}
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if (a.err != nil) != (b.err != nil) {
			return a.err == nil
		}
		if a.err != nil {
			return a.name < b.name
		}
		if c.groupDirs && a.isDir != b.isDir {
			return a.isDir
		}
		switch {
		case c.sizeSort:
			if a.info.Size() != b.info.Size() {
				if c.reverse {
					return a.info.Size() < b.info.Size()
				}
				return a.info.Size() > b.info.Size()
			}
		case c.timeSort:
			at, bt := c.selectedTime(a.lsInfo), c.selectedTime(b.lsInfo)
			if at != bt {
				if c.reverse {
					return at < bt
				}
				return at > bt
			}
		case c.extension:
			ae, be := filepath.Ext(a.name), filepath.Ext(b.name)
			if ae != be {
				if c.reverse {
					return ae > be
				}
				return ae < be
			}
		case c.version:
			if n := versionCompare(a.name, b.name); n != 0 {
				return n < 0 != c.reverse
			}
		}
		if c.reverse {
			return a.name > b.name
		}
		return a.name < b.name
	})
}

func versionCompare(a, b string) int {
	for len(a) > 0 && len(b) > 0 {
		ra, sa := utf8.DecodeRuneInString(a)
		rb, sb := utf8.DecodeRuneInString(b)
		if ra >= '0' && ra <= '9' && rb >= '0' && rb <= '9' {
			ia, ib := 0, 0
			for ia < len(a) && a[ia] >= '0' && a[ia] <= '9' {
				ia++
			}
			for ib < len(b) && b[ib] >= '0' && b[ib] <= '9' {
				ib++
			}
			na, nb := strings.TrimLeft(a[:ia], "0"), strings.TrimLeft(b[:ib], "0")
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			if ia != ib {
				if ia < ib {
					return -1
				}
				return 1
			}
			a, b = a[ia:], b[ib:]
			continue
		}
		if ra != rb {
			if ra < rb {
				return -1
			}
			return 1
		}
		a, b = a[sa:], b[sb:]
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

func (c cmd) readDirectory(ctx context.Context, dir string) ([]file, error) {
	resolved := c.resolve(dir)
	f, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	entries, readErr := f.ReadDir(-1)
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	files := make([]file, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		name := entry.Name()
		if !c.shouldShow(name) {
			continue
		}
		path := filepath.Join(resolved, name)
		info, err := entry.Info()
		if err != nil {
			files = append(files, file{path: path, name: name, err: err})
			continue
		}
		files = append(files, c.makeFile(path, name, info))
	}
	if c.all {
		for _, dot := range []struct {
			name string
			path string
		}{
			{name: ".", path: resolved},
			{name: "..", path: filepath.Join(resolved, "..")},
		} {
			info, err := os.Lstat(dot.path)
			if err != nil {
				files = append(files, file{name: dot.name, path: dot.path, err: err})
				continue
			}
			files = append(files, c.makeFile(dot.path, dot.name, info))
		}
	}
	c.sortFiles(files)
	return files, nil
}

func (c cmd) readOperand(path string) file {
	resolved := c.resolve(path)
	info, err := os.Lstat(resolved)
	if err != nil {
		return file{name: path, path: resolved, err: err}
	}
	if c.followCmdLinks && info.Mode()&os.ModeSymlink != 0 && !c.followLinks {
		info, err = os.Stat(resolved)
		if err != nil {
			return file{name: path, path: resolved, err: err}
		}
	}
	f := c.makeFile(resolved, path, info)
	return f
}

func (c cmd) quoteName(name string) string {
	switch c.quoteStyle {
	case "literal":
		return name
	case "c", "clocale", "shell", "shell-always", "shell-escape", "shell-escape-always":
		return strconv.Quote(name)
	case "escape":
		return strings.Trim(strconv.Quote(name), "\"")
	}
	if c.quoted {
		return strconv.Quote(name)
	}
	if c.escape {
		return strings.Trim(strconv.Quote(name), "\"")
	}
	if c.quiet {
		var b strings.Builder
		for _, r := range name {
			if unicode.IsControl(r) {
				b.WriteRune('?')
			} else {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	if c.rawNames {
		return name
	}
	return ls.FileInfo{Name: name}.PrintableName()
}

func (c cmd) renderName(f file) string {
	return c.decorate(c.quoteName(f.name), f)
}

func (c cmd) printLong(f file) string {
	info := f.lsInfo
	info.Name = c.renderName(f)
	if c.timeMode != "modify" {
		if t := reflectedTime(info, map[string]string{"atime": "ATime", "ctime": "CTime", "status": "CTime"}[c.timeMode]); !t.IsZero() {
			info.MTime = t
		}
	}
	var s string
	if c.long {
		s = ls.LongStringer{Human: c.human, Name: literalNameStringer{}}.FileString(info)
		fields := strings.SplitN(s, "\t", 6)
		hasLinks := hasReflectedField(info, "NLink")
		if hasLinks && len(fields) > 1 {
			fields = append(fields[:1], append([]string{strconv.FormatInt(reflectedInt(info, "NLink"), 10)}, fields[1:]...)...)
		}
		ownerColumn, groupColumn := 1, 2
		if hasLinks {
			ownerColumn, groupColumn = 2, 3
		}
		if c.numericOwners && ownerColumn < len(fields)-1 {
			fields[ownerColumn] = strconv.FormatInt(reflectedInt(info, "UID"), 10)
		}
		if c.numericOwners && hasReflectedField(info, "GID") && groupColumn < len(fields)-1 {
			fields[groupColumn] = strconv.FormatInt(reflectedInt(info, "GID"), 10)
		}
		if c.omitUser && ownerColumn < len(fields)-1 {
			fields = append(fields[:ownerColumn], fields[ownerColumn+1:]...)
			if groupColumn > ownerColumn {
				groupColumn--
			}
		}
		if c.omitGroup && hasReflectedField(info, "GID") && groupColumn < len(fields)-1 {
			fields = append(fields[:groupColumn], fields[groupColumn+1:]...)
		}
		if c.fullTime || c.timeStyle != "" && c.timeStyle != "locale" {
			if len(fields) > 1 {
				fields[len(fields)-2] = info.MTime.Format(timeLayout(c.timeStyle, c.fullTime))
			}
		}
		s = strings.Join(fields, "\t")
	} else {
		s = c.renderName(f)
	}
	if c.showInode {
		s = fmt.Sprintf("%d\t%s", reflectedInt(info, "Ino"), s)
	}
	if c.showBlocks {
		blocks := reflectedInt(info, "Blocks")
		if !hasReflectedField(info, "Blocks") && f.info != nil {
			blocks = (f.info.Size() + 511) / 512
		}
		if c.blockSizeK {
			blocks = (blocks + 1) / 2
		}
		if c.human {
			s = fmt.Sprintf("%s\t%s", humanBlockSize(blocks, c.blockSizeK), s)
		} else {
			s = fmt.Sprintf("%d\t%s", blocks, s)
		}
	}
	if c.classify || c.fileType || c.dirsOnly {
		mark := indicator(info)
		if c.fileType {
			mark = fileTypeIndicator(info)
		} else if c.dirsOnly && !info.Mode.IsDir() {
			mark = ""
		}
		s += mark
	}
	return s
}

func fileTypeIndicator(fi ls.FileInfo) string {
	switch {
	case fi.Mode&os.ModeDir != 0:
		return "/"
	case fi.Mode&os.ModeSymlink != 0:
		return "@"
	case fi.Mode&os.ModeSocket != 0:
		return "="
	case fi.Mode&os.ModeNamedPipe != 0:
		return "|"
	default:
		return ""
	}
}

func (c cmd) printError(f file) {
	if c.errW == nil {
		return
	}
	fmt.Fprintf(c.errW, "ls: cannot access %q: %v\n", f.name, f.err)
}

func (c cmd) writeEntries(files []file) {
	if c.long || c.single || (!c.terminal && !c.columns && !c.across && !c.comma) {
		for _, f := range files {
			if f.err != nil {
				c.printError(f)
				continue
			}
			fmt.Fprintln(c.w, c.printLong(f))
		}
		return
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		if f.err != nil {
			c.printError(f)
			continue
		}
		names = append(names, c.printLong(f))
	}
	if c.comma {
		for i, name := range names {
			if i > 0 {
				fmt.Fprint(c.w, ", ")
			}
			fmt.Fprint(c.w, name)
		}
		fmt.Fprintln(c.w)
		return
	}
	width := c.width
	if width <= 0 {
		width = 80
	}
	writeColumns(c.w, names, width, c.across)
}

func (c cmd) listDirectory(ctx context.Context, dir string, header bool, ancestors map[string]bool) (bool, error) {
	resolved := c.resolve(dir)
	if c.recurse {
		key := c.directoryIdentity(resolved)
		if ancestors[key] {
			return true, fmt.Errorf("already visited directory %q", dir)
		}
		ancestors[key] = true
		defer delete(ancestors, key)
	}
	files, err := c.readDirectory(ctx, dir)
	if err != nil {
		c.printError(file{name: dir, err: err})
		return true, nil
	}
	if header {
		fmt.Fprintf(c.w, "%s:\n", c.quoteName(dir))
	}
	c.writeEntries(files)
	hasErr := false
	for _, f := range files {
		hasErr = hasErr || f.err != nil
	}
	if c.recurse {
		children := make([]file, 0)
		for _, f := range files {
			if f.err == nil && f.isDir && f.name != "." && f.name != ".." {
				children = append(children, f)
			}
		}
		for _, child := range children {
			fmt.Fprintln(c.w)
			childPath := filepath.Join(dir, child.name)
			childHeader := !c.directory
			childErr, err := c.listDirectory(ctx, childPath, childHeader, ancestors)
			if err != nil {
				return true, err
			}
			hasErr = hasErr || childErr
		}
	}
	return hasErr, nil
}

func (c cmd) directoryIdentity(path string) string {
	if info, err := os.Stat(path); err == nil {
		metadata := ls.FromOSFileInfo(path, info)
		if inode := reflectedInt(metadata, "Ino"); inode != 0 {
			return fmt.Sprintf("inode:%d:%d", reflectedInt(metadata, "Dev"), inode)
		}
	}
	if canonical, err := filepath.EvalSymlinks(path); err == nil {
		return canonical
	}
	return filepath.Clean(path)
}

func (c cmd) list(ctx context.Context, names []string) (bool, error) {
	if len(names) == 0 {
		names = []string{"."}
	}
	operands := make([]file, 0, len(names))
	for _, name := range names {
		operands = append(operands, c.readOperand(name))
	}
	c.sortFiles(operands)
	hasError := false
	directories := make([]file, 0)
	printedOperand := false
	for _, f := range operands {
		if f.err != nil {
			c.printError(f)
			hasError = true
			continue
		}
		if f.isDir && !c.directory {
			directories = append(directories, f)
			continue
		}
		c.writeEntries([]file{f})
		printedOperand = true
	}
	needHeader := len(operands) > 1 || c.recurse
	ancestors := make(map[string]bool)
	for i, dir := range directories {
		if i > 0 || printedOperand && needHeader {
			fmt.Fprintln(c.w)
		}
		hadErr, err := c.listDirectory(ctx, dir.name, needHeader, ancestors)
		if err != nil {
			return true, fmt.Errorf("error while listing %q: %w", dir.name, err)
		}
		hasError = hasError || hadErr
	}
	return hasError, nil
}

func (c cmd) listName(stringer ls.Stringer, name string, prefix bool) error {
	resolved := c.resolve(name)
	info, err := os.Lstat(resolved)
	if err != nil {
		fmt.Fprintln(c.w, err)
		return nil
	}
	if !info.IsDir() || c.directory {
		entry := c.makeFile(resolved, name, info)
		fmt.Fprintln(c.w, stringer.FileString(entry.lsInfo))
		return nil
	}
	files, err := c.readDirectory(context.Background(), name)
	if err != nil {
		fmt.Fprintln(c.w, err)
		return nil
	}
	if prefix {
		fmt.Fprintf(c.w, "%s:\n", c.quoteName(name))
	}
	for _, entry := range files {
		if entry.err != nil {
			fmt.Fprintln(c.w, entry.err)
			continue
		}
		if c.classify {
			entry.lsInfo.Name += indicator(entry.lsInfo)
		}
		fmt.Fprintln(c.w, stringer.FileString(entry.lsInfo))
	}
	return nil
}

func newFlagSet(name string, c *cmd, output io.Writer) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(output)
	f.BoolVar(&c.all, "a", false, "show hidden files, including . and ..")
	f.BoolVar(&c.almostAll, "A", false, "show hidden files except . and ..")
	f.BoolVar(&c.backup, "B", false, "do not list implied entries ending with ~")
	f.BoolVar(&c.human, "h", false, "human-readable sizes")
	f.BoolVar(&c.directory, "d", false, "list directories themselves, not their contents")
	f.BoolVar(&c.long, "l", false, "use long listing format")
	f.BoolVar(&c.quoted, "Q", false, "enclose entry names in double quotes")
	f.StringVar(&c.quoteStyle, "quoting-style", "", "use the given quoting style")
	f.BoolVar(&c.quiet, "q", false, "print ? instead of non-printable characters")
	f.BoolVar(&c.escape, "b", false, "print C-style escapes for non-graphic characters")
	f.BoolVar(&c.rawNames, "N", false, "print entry names without quoting")
	f.BoolVar(&c.recurse, "R", false, "list subdirectories recursively")
	f.BoolVar(&c.classify, "F", false, "append an indicator to entries")
	f.BoolVar(&c.fileType, "file-type", false, "append an indicator except for executable files")
	f.BoolVar(&c.dirsOnly, "p", false, "append / to directories")
	f.BoolVar(&c.sizeSort, "S", false, "sort by file size")
	f.BoolVar(&c.timeSort, "t", false, "sort by modification time")
	f.BoolVar(&c.extension, "X", false, "sort alphabetically by entry extension")
	f.BoolVar(&c.version, "v", false, "natural sort of (version) numbers")
	f.BoolVar(&c.reverse, "r", false, "reverse the sort order")
	f.BoolVar(&c.noSort, "U", false, "do not sort")
	f.BoolVar(&c.force, "f", false, "do not sort, enable -a, and disable -l/-s")
	f.BoolVar(&c.groupDirs, "group-directories-first", false, "group directories before files")
	f.BoolVar(&c.showInode, "i", false, "print the index number of each file")
	f.BoolVar(&c.showBlocks, "s", false, "print the allocated size of each file, in blocks")
	f.BoolVar(&c.blockSizeK, "k", false, "use 1 KiB blocks for -s")
	f.BoolVar(&c.comma, "m", false, "fill width with a comma-separated list of entries")
	f.BoolVar(&c.columns, "C", false, "list entries by columns")
	f.BoolVar(&c.across, "x", false, "list entries by lines instead of by columns")
	f.BoolVar(&c.single, "1", false, "list one file per line")
	f.BoolVar(&c.followLinks, "L", false, "follow symbolic links")
	f.BoolVar(&c.followCmdLinks, "H", false, "follow command-line symbolic links")
	f.BoolVar(&c.omitUser, "g", false, "use long format without owner information")
	f.BoolVar(&c.omitGroup, "G", false, "do not print group names")
	f.BoolVar(&c.omitGroup, "o", false, "use long format without group information")
	f.BoolVar(&c.numericOwners, "n", false, "like -l, but list numeric user and group IDs")
	f.StringVar(&c.colorMode, "color", "auto", "colorize output: always, auto, or never")
	f.StringVar(&c.iconMode, "icons", "never", "show icons: always, auto, or never")
	f.StringVar(&c.linkMode, "hyperlink", "never", "hyperlink names: always, auto, or never")
	f.Var((*stringList)(&c.ignore), "ignore", "do not list entries matching the pattern")
	f.Var((*stringList)(&c.hide), "hide", "do not list implied entries matching the pattern")
	f.StringVar(&c.timeMode, "time", "modify", "select time: atime, ctime, or modify")
	f.StringVar(&c.timeStyle, "time-style", "locale", "use locale, iso, long-iso, or full-iso timestamps")
	f.BoolVar(&c.fullTime, "full-time", false, "show full date and time")
	f.StringVar(&c.formatMode, "format", "", "choose vertical, across, commas, single-column, or long format")
	f.StringVar(&c.sortMode, "sort", "", "sort by none, size, time, version, extension, or name")
	f.StringVar(&c.indicatorStyle, "indicator-style", "", "append indicators: none, slash, file-type, or classify")
	f.IntVar(&c.width, "w", c.width, "set output width")
	f.IntVar(&c.width, "width", c.width, "set output width")
	f.BoolVar(&c.timeModeAtime, "u", false, "use access time")
	f.BoolVar(&c.timeModeCtime, "c", false, "use status-change time")
	f.Var((*stringList)(&c.ignore), "I", "do not list entries matching the pattern")
	f.Usage = func() {
		fmt.Fprintf(f.Output(), "Usage: ls [OPTION]... [FILE]...\n\nOptions:\n")
		f.PrintDefaults()
	}
	return f
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func parseAndRun(ctx context.Context, c *cmd, args []string, output io.Writer) int {
	args = normalizeOptionalModes(args)
	f := newFlagSet("ls", c, output)
	if err := f.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if c.timeModeAtime {
		c.timeMode = "atime"
	}
	if c.timeModeCtime {
		c.timeMode = "ctime"
	}
	if c.omitUser || c.omitGroup || c.numericOwners {
		c.long = true
	}
	switch c.sortMode {
	case "":
	case "name":
		c.noSort, c.sizeSort, c.timeSort, c.extension, c.version = false, false, false, false, false
	case "none":
		c.noSort, c.sizeSort, c.timeSort, c.extension, c.version = true, false, false, false, false
	case "size":
		c.noSort, c.sizeSort, c.timeSort, c.extension, c.version = false, true, false, false, false
	case "time":
		c.noSort, c.sizeSort, c.timeSort, c.extension, c.version = false, false, true, false, false
	case "extension":
		c.noSort, c.sizeSort, c.timeSort, c.extension, c.version = false, false, false, true, false
	case "version":
		c.noSort, c.sizeSort, c.timeSort, c.extension, c.version = false, false, false, false, true
	default:
		fmt.Fprintf(output, "ls: invalid sort type %q\n", c.sortMode)
		return 2
	}
	if c.force {
		c.noSort = true
		c.all = true
		c.long = false
		c.showBlocks = false
	}
	if c.sizeSort || c.timeSort || c.extension || c.version {
		c.noSort = false
	}
	switch c.timeMode {
	case "access", "use":
		c.timeMode = "atime"
	case "change":
		c.timeMode = "ctime"
	case "modification":
		c.timeMode = "modify"
	}
	if c.timeMode != "atime" && c.timeMode != "ctime" && c.timeMode != "status" && c.timeMode != "modify" {
		fmt.Fprintf(output, "ls: invalid time style %q\n", c.timeMode)
		return 2
	}
	switch c.indicatorStyle {
	case "":
	case "none":
		c.classify, c.fileType, c.dirsOnly = false, false, false
	case "slash":
		c.classify, c.fileType, c.dirsOnly = false, false, true
	case "file-type":
		c.classify, c.fileType, c.dirsOnly = false, true, false
	case "classify":
		c.classify, c.fileType, c.dirsOnly = true, false, false
	default:
		fmt.Fprintf(output, "ls: invalid indicator style %q\n", c.indicatorStyle)
		return 2
	}
	if c.fullTime {
		c.timeStyle = "full-iso"
		c.long = true
	}
	switch c.timeStyle {
	case "locale", "iso", "long-iso", "full-iso":
	default:
		fmt.Fprintf(output, "ls: invalid time style %q\n", c.timeStyle)
		return 2
	}
	switch c.formatMode {
	case "":
	case "vertical":
		c.columns, c.across, c.comma, c.single, c.long = true, false, false, false, false
	case "across", "horizontal":
		c.columns, c.across, c.comma, c.single, c.long = false, true, false, false, false
	case "commas":
		c.columns, c.across, c.comma, c.single, c.long = false, false, true, false, false
	case "single-column":
		c.columns, c.across, c.comma, c.single, c.long = false, false, false, true, false
	case "long", "verbose":
		c.columns, c.across, c.comma, c.single, c.long = false, false, false, false, true
	default:
		fmt.Fprintf(output, "ls: invalid format %q\n", c.formatMode)
		return 2
	}
	if !validMode(c.colorMode) || !validMode(c.iconMode) || !validMode(c.linkMode) {
		fmt.Fprintln(output, "ls: expected always, auto, or never for color, icons, and hyperlinks")
		return 2
	}
	if c.quoteStyle != "" && !validQuotingStyle(c.quoteStyle) {
		fmt.Fprintf(output, "ls: invalid quoting style %q\n", c.quoteStyle)
		return 2
	}
	names := f.Args()
	hadError, err := c.list(ctx, names)
	if err != nil {
		fmt.Fprintf(output, "ls: %v\n", err)
		return 1
	}
	if hadError {
		return 1
	}
	return 0
}

func normalizeOptionalModes(args []string) []string {
	normalized := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--" {
			return append(normalized, args[len(normalized):]...)
		}
		switch arg {
		case "--color":
			arg = "--color=always"
		case "--icons":
			arg = "--icons=auto"
		case "--hyperlink":
			arg = "--hyperlink=auto"
		}
		normalized = append(normalized, arg)
	}
	return normalized
}

func validMode(value string) bool {
	return value == "always" || value == "auto" || value == "never"
}

func validQuotingStyle(value string) bool {
	switch value {
	case "literal", "shell", "shell-always", "shell-escape", "shell-escape-always", "c", "escape", "clocale", "locale":
		return true
	default:
		return false
	}
}

func run(w io.Writer, args []string) error {
	if len(args) == 0 {
		args = []string{"ls"}
	}
	c := cmd{w: w, errW: w, colorMode: "never", iconMode: "never", linkMode: "never", timeMode: "modify"}
	if parseAndRun(context.Background(), &c, args[1:], w) != 0 {
		return fmt.Errorf("ls failed")
	}
	return nil
}

// Main runs ls reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) int {
	if ioEnv.Stdout == nil {
		ioEnv.Stdout = io.Discard
	}
	if ioEnv.Stderr == nil {
		ioEnv.Stderr = io.Discard
	}
	c := cmd{
		w:           ioEnv.Stdout,
		errW:        ioEnv.Stderr,
		terminal:    ioEnv.StdoutIsTerminal,
		width:       ioEnv.TerminalWidth,
		colorMode:   "auto",
		iconMode:    "never",
		linkMode:    "never",
		timeMode:    "modify",
		resolvePath: ioEnv.ResolvePath,
		env:         ioEnv.Getenv,
	}
	if _, ok := lookupEnv(ioEnv, "NO_COLOR"); ok {
		c.noColor = true
		c.colorMode = "never"
	}
	if c.env != nil {
		if c.env("TERM") == "dumb" {
			c.colorMode = "never"
		}
	}
	return parseAndRun(ctx, &c, args, ioEnv.Stderr)
}

func lookupEnv(ioEnv core.IO, key string) (string, bool) {
	if ioEnv.LookupEnv != nil {
		return ioEnv.LookupEnv(key)
	}
	return "", false
}
