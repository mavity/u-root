// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// cp copies files and directories.
package cpcmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/cp"
)

var errUsage = errors.New("usage: cp [OPTION]... SOURCE... DEST")
var errSameFile = errors.New("source and destination are the same file")

type symlinkFlag struct {
	mode   *cp.SymlinkMode
	target cp.SymlinkMode
}

func (f symlinkFlag) Set(value string) error {
	if value == "false" {
		return nil
	}
	if value != "true" {
		return fmt.Errorf("option does not accept a value")
	}
	*f.mode = f.target
	return nil
}

func (f symlinkFlag) String() string   { return "" }
func (f symlinkFlag) IsBoolFlag() bool { return true }
func (f symlinkFlag) Type() string     { return "bool" }

type boolActionFlag struct {
	set func(bool)
}

func (f boolActionFlag) Set(value string) error {
	enabled := value == "true"
	if !enabled && value != "false" {
		return fmt.Errorf("invalid boolean value %q", value)
	}
	f.set(enabled)
	return nil
}

func (f boolActionFlag) String() string   { return "false" }
func (f boolActionFlag) IsBoolFlag() bool { return true }
func (f boolActionFlag) Type() string     { return "bool" }

type flags struct {
	recursive            bool
	archive              bool
	interactive          bool
	force                bool
	noClobber            bool
	verbose              bool
	noTargetDirectory    bool
	targetDirectory      string
	targetDirectorySet   bool
	parents              bool
	stripTrailingSlashes bool
	removeDestination    bool
	keepDirectorySymlink bool
	oneFileSystem        bool
	copyContents         bool
	attributesOnly       bool
	hardLink             bool
	symbolicLink         bool
	debug                bool
	help                 bool
	version              bool
	dereferencePreserve  bool
	symlinks             cp.SymlinkMode
	preserve             string
	noPreserve           string
	backup               string
	backupSuffix         string
	update               string
	reflink              string
	sparse               string
	contextLabel         string
}

func promptOverwrite(dst string, out io.Writer, in *bufio.Reader) (bool, error) {
	if _, err := fmt.Fprintf(out, "cp: overwrite %q? ", dst); err != nil {
		return false, err
	}
	answer, err := in.ReadString('\n')
	if err != nil {
		return false, err
	}
	return len(answer) > 0 && strings.EqualFold(answer[:1], "y"), nil
}

func setupPreCallback(f flags, update cp.UpdateMode, writer io.Writer, reader *bufio.Reader) func(string, string, os.FileInfo) error {
	return func(src, dst string, srcInfo os.FileInfo) error {
		if !f.recursive && !f.archive && srcInfo.IsDir() {
			fmt.Fprintf(writer, "cp: -r not specified, omitting directory %s\n", src)
			return fmt.Errorf("omitting directory %q", src)
		}

		dstInfo, err := os.Lstat(dst)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot access %q: %w", dst, err)
		}

		same := os.SameFile(srcInfo, dstInfo)
		if srcInfo.Mode()&os.ModeSymlink == 0 && dstInfo.Mode()&os.ModeSymlink != 0 {
			if followed, statErr := os.Stat(dst); statErr == nil {
				same = same || os.SameFile(srcInfo, followed)
			}
		}
		if same && !(f.force && f.backup != "" && f.backup != "none" && f.backup != "off") {
			return fmt.Errorf("%w: %q and %q", errSameFile, src, dst)
		}
		if srcInfo.IsDir() && dstInfo.IsDir() {
			return nil
		}

		if f.noClobber || update == cp.UpdateNone || update == cp.UpdateNoneFail {
			if update == cp.UpdateNoneFail {
				return fmt.Errorf("not overwriting %q", dst)
			}
			return cp.ErrSkip
		}
		if update == cp.UpdateOlder && !srcInfo.ModTime().After(dstInfo.ModTime()) {
			return cp.ErrSkip
		}
		if f.interactive {
			overwrite, err := promptOverwrite(dst, writer, reader)
			if err != nil {
				return err
			}
			if !overwrite {
				return cp.ErrSkip
			}
		}
		return nil
	}
}

func parseAttributes(value string) (cp.Attributes, error) {
	var attrs cp.Attributes
	if value == "default" {
		return cp.DefaultAttributes, nil
	}
	if value == "all" {
		return cp.AllAttributes, nil
	}
	for _, name := range strings.Split(value, ",") {
		switch name {
		case "mode":
			attrs.Mode = true
		case "ownership":
			attrs.Ownership = true
		case "timestamps":
			attrs.Timestamps = true
		case "links":
			attrs.Links = true
		case "xattr":
			attrs.Xattrs = true
		case "context":
			attrs.Context = true
		default:
			return cp.Attributes{}, fmt.Errorf("invalid attribute %q", name)
		}
	}
	return attrs, nil
}

func addAttributes(dst *cp.Attributes, src cp.Attributes) {
	dst.Mode = dst.Mode || src.Mode
	dst.Ownership = dst.Ownership || src.Ownership
	dst.Timestamps = dst.Timestamps || src.Timestamps
	dst.Links = dst.Links || src.Links
	dst.Xattrs = dst.Xattrs || src.Xattrs
	dst.Context = dst.Context || src.Context
}

func removeAttributes(dst *cp.Attributes, src cp.Attributes) {
	if src.Mode {
		dst.Mode = false
	}
	if src.Ownership {
		dst.Ownership = false
	}
	if src.Timestamps {
		dst.Timestamps = false
	}
	if src.Links {
		dst.Links = false
	}
	if src.Xattrs {
		dst.Xattrs = false
	}
	if src.Context {
		dst.Context = false
	}
}

func parseBackupMode(value string) (cp.BackupMode, error) {
	switch value {
	case "", "existing", "nil":
		if control := os.Getenv("VERSION_CONTROL"); control != "" && control != "existing" && control != "nil" {
			return parseBackupMode(control)
		}
		return cp.BackupExisting, nil
	case "numbered", "t":
		return cp.BackupNumbered, nil
	case "simple", "never":
		return cp.BackupSimple, nil
	case "none", "off":
		return cp.BackupNone, nil
	default:
		return cp.BackupNone, fmt.Errorf("invalid backup type %q", value)
	}
}

func parseUpdateMode(value string) (cp.UpdateMode, error) {
	switch value {
	case "":
		return cp.UpdateAll, nil
	case "older":
		return cp.UpdateOlder, nil
	case "all":
		return cp.UpdateAll, nil
	case "none":
		return cp.UpdateNone, nil
	case "none-fail":
		return cp.UpdateNoneFail, nil
	default:
		return cp.UpdateAll, fmt.Errorf("invalid update type %q", value)
	}
}

func parseReflinkMode(value string) (cp.ReflinkMode, error) {
	switch value {
	case "":
		return cp.ReflinkAuto, nil
	case "always":
		return cp.ReflinkAlways, nil
	case "auto":
		return cp.ReflinkAuto, nil
	case "never":
		return cp.ReflinkNever, nil
	default:
		return cp.ReflinkAuto, fmt.Errorf("invalid reflink type %q", value)
	}
}

func parseSparseMode(value string) (cp.SparseMode, error) {
	switch value {
	case "", "auto":
		return cp.SparseAuto, nil
	case "always":
		return cp.SparseAlways, nil
	case "never":
		return cp.SparseNever, nil
	default:
		return cp.SparseAuto, fmt.Errorf("invalid sparse type %q", value)
	}
}

func parseArgs(args []string, out, errw io.Writer) (flags, []string, error) {
	var f flags
	fs := pflag.NewFlagSet("cp", pflag.ContinueOnError)
	fs.SetOutput(errw)

	fs.VarP(boolActionFlag{func(enabled bool) {
		f.archive = enabled
		if enabled {
			f.symlinks = cp.SymlinkNever
		}
	}}, "archive", "a", "same as -dR --preserve=all")
	fs.Lookup("archive").NoOptDefVal = "true"
	fs.BoolVarP(&f.recursive, "recursive", "R", false, "copy directories recursively")
	fs.BoolVarP(&f.recursive, "r", "r", false, "copy directories recursively")
	fs.VarP(symlinkFlag{&f.symlinks, cp.SymlinkCommandLine}, "dereference-command-line", "H", "follow command-line symbolic links")
	fs.VarP(symlinkFlag{&f.symlinks, cp.SymlinkAlways}, "dereference", "L", "always follow symbolic links")
	fs.VarP(symlinkFlag{&f.symlinks, cp.SymlinkNever}, "no-dereference", "P", "never follow symbolic links")
	fs.Lookup("dereference-command-line").NoOptDefVal = "true"
	fs.Lookup("dereference").NoOptDefVal = "true"
	fs.Lookup("no-dereference").NoOptDefVal = "true"
	fs.VarP(boolActionFlag{func(enabled bool) {
		f.dereferencePreserve = enabled
		if enabled {
			f.symlinks = cp.SymlinkNever
		}
	}}, "no-dereference-preserve-links", "d", "preserve symbolic links and hard links")
	fs.Lookup("no-dereference-preserve-links").NoOptDefVal = "true"

	fs.VarP(boolActionFlag{func(enabled bool) {
		f.interactive = enabled
		if enabled {
			f.noClobber = false
		}
	}}, "interactive", "i", "prompt before overwrite")
	fs.Lookup("interactive").NoOptDefVal = "true"
	fs.BoolVarP(&f.force, "force", "f", false, "remove destination and retry if it cannot be opened")
	fs.VarP(boolActionFlag{func(enabled bool) {
		f.noClobber = enabled
		if enabled {
			f.interactive = false
		}
	}}, "no-clobber", "n", "do not overwrite an existing file")
	fs.Lookup("no-clobber").NoOptDefVal = "true"
	fs.BoolVarP(&f.verbose, "verbose", "v", false, "explain what is being done")
	fs.BoolVarP(&f.hardLink, "link", "l", false, "hard link files instead of copying")
	fs.BoolVarP(&f.symbolicLink, "symbolic-link", "s", false, "make symbolic links instead of copying")
	fs.BoolVarP(&f.noTargetDirectory, "no-target-directory", "T", false, "treat DEST as a normal file")
	fs.StringVarP(&f.targetDirectory, "target-directory", "t", "", "copy all SOURCE arguments into DIRECTORY")

	fs.StringVarP(&f.backup, "backup", "b", "", "make a backup of each existing destination")
	fs.Lookup("backup").NoOptDefVal = "existing"
	fs.StringVarP(&f.preserve, "preserve", "p", "", "preserve specified attributes")
	fs.Lookup("preserve").NoOptDefVal = "default"
	fs.StringVar(&f.noPreserve, "no-preserve", "", "do not preserve specified attributes")
	fs.StringVarP(&f.update, "update", "u", "", "control which existing files are replaced")
	fs.Lookup("update").NoOptDefVal = "older"
	fs.StringVarP(&f.backupSuffix, "suffix", "S", "", "override the usual backup suffix")
	fs.StringVar(&f.reflink, "reflink", "", "control clone/reflink copies")
	fs.Lookup("reflink").NoOptDefVal = "always"
	fs.StringVar(&f.sparse, "sparse", "", "control creation of sparse files")
	fs.StringVarP(&f.contextLabel, "context", "Z", "", "set SELinux context; use policy default if omitted")
	fs.Lookup("context").NoOptDefVal = "default"

	fs.BoolVar(&f.parents, "parents", false, "use full source file name under DIRECTORY")
	fs.BoolVar(&f.parents, "path", false, "deprecated alias for --parents")
	fs.BoolVar(&f.stripTrailingSlashes, "strip-trailing-slashes", false, "remove trailing slashes from SOURCE")
	fs.BoolVar(&f.removeDestination, "remove-destination", false, "remove each existing destination before opening")
	fs.BoolVar(&f.keepDirectorySymlink, "keep-directory-symlink", false, "follow existing symlinks to directories")
	fs.BoolVarP(&f.oneFileSystem, "one-file-system", "x", false, "stay on this file system")
	fs.BoolVar(&f.copyContents, "copy-contents", false, "copy contents of special files when recursive")
	fs.BoolVar(&f.attributesOnly, "attributes-only", false, "copy only attributes, not file contents")
	fs.BoolVar(&f.debug, "debug", false, "explain how each file is copied")
	fs.BoolVar(&f.help, "help", false, "display this help and exit")
	fs.BoolVar(&f.version, "version", false, "output version information and exit")

	fs.Usage = func() {
		fmt.Fprintln(out, "Usage: cp [OPTION]... SOURCE... DEST")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return f, nil, err
	}
	fs.SetOutput(out)
	f.targetDirectorySet = fs.Changed("target-directory")
	f.verbose = f.verbose || f.debug
	if f.help {
		fs.Usage()
	}
	return f, fs.Args(), nil
}

func sourcePathArgument(path string, stripTrailing bool) string {
	if stripTrailing {
		trimmed := strings.TrimRight(path, `/\`)
		if trimmed != "" {
			return trimmed
		}
	}
	return path
}

func parentRelativePath(src string) string {
	clean := filepath.Clean(src)
	volume := filepath.VolumeName(clean)
	clean = strings.TrimPrefix(clean, volume)
	clean = strings.TrimLeft(clean, string(filepath.Separator))
	clean = strings.TrimPrefix(clean, "."+string(filepath.Separator))
	return clean
}

func runWithCtx(ctx context.Context, args []string, out, errw io.Writer, input *bufio.Reader, resolvePath func(string) string) error {
	f, operands, err := parseArgs(args, out, errw)
	if err != nil {
		return err
	}
	if f.help {
		return nil
	}
	if f.version {
		_, err := fmt.Fprintln(out, "cp (u-root)")
		return err
	}
	if f.noTargetDirectory && f.targetDirectorySet {
		return fmt.Errorf("--target-directory and --no-target-directory are mutually exclusive")
	}
	if f.hardLink && f.symbolicLink {
		return fmt.Errorf("--link and --symbolic-link are mutually exclusive")
	}
	if f.noClobber && f.backup != "" && f.backup != "none" && f.backup != "off" {
		return fmt.Errorf("--backup and --no-clobber are mutually exclusive")
	}

	if !f.targetDirectorySet && len(operands) < 2 {
		return errUsage
	}
	sources := operands
	rawDest := f.targetDirectory
	if !f.targetDirectorySet {
		sources, rawDest = operands[:len(operands)-1], operands[len(operands)-1]
	}
	if len(sources) == 0 {
		return errUsage
	}

	backupMode, err := parseBackupMode(f.backup)
	if err != nil {
		return err
	}
	updateMode, err := parseUpdateMode(f.update)
	if err != nil {
		return err
	}
	reflinkMode, err := parseReflinkMode(f.reflink)
	if err != nil {
		return err
	}
	sparseMode, err := parseSparseMode(f.sparse)
	if err != nil {
		return err
	}

	preserve := cp.Attributes{}
	if f.archive {
		preserve = cp.AllAttributes
	}
	if f.dereferencePreserve {
		preserve.Links = true
	}
	if f.preserve != "" {
		attrs, err := parseAttributes(f.preserve)
		if err != nil {
			return err
		}
		addAttributes(&preserve, attrs)
	}
	if f.noPreserve != "" {
		attrs, err := parseAttributes(f.noPreserve)
		if err != nil {
			return err
		}
		removeAttributes(&preserve, attrs)
	}

	symlinkMode := f.symlinks
	if f.dereferencePreserve && symlinkMode == cp.SymlinkDefault {
		symlinkMode = cp.SymlinkNever
	}
	if f.archive && symlinkMode == cp.SymlinkDefault {
		symlinkMode = cp.SymlinkNever
	}
	if !f.archive && f.dereferencePreserve && f.symlinks == cp.SymlinkDefault {
		symlinkMode = cp.SymlinkNever
	}

	resolvedDest := rawDest
	if resolvePath != nil {
		resolvedDest = resolvePath(rawDest)
	}
	destStat, destStatErr := os.Stat(resolvedDest)
	destIsDir := destStatErr == nil && destStat.IsDir()
	destWasDir := destIsDir
	if f.targetDirectorySet {
		if destStatErr != nil {
			return fmt.Errorf("target directory %q: %w", rawDest, destStatErr)
		}
		if !destIsDir {
			return fmt.Errorf("target %q is not a directory", rawDest)
		}
		destIsDir = true
	} else if destStatErr != nil && !errors.Is(destStatErr, os.ErrNotExist) {
		return fmt.Errorf("cannot access %q: %w", rawDest, destStatErr)
	}
	if f.noTargetDirectory {
		destIsDir = false
	}
	if len(sources) > 1 && !destIsDir {
		return eNotDir
	}
	if f.parents && !destIsDir {
		return fmt.Errorf("--parents requires a target directory")
	}

	var reader bufio.Reader
	if input != nil {
		reader = *input
	} else {
		reader = *bufio.NewReader(strings.NewReader(""))
	}
	options := cp.Options{
		Context:              ctx,
		State:                &cp.State{},
		Recursive:            f.recursive || f.archive,
		Symlinks:             symlinkMode,
		Preserve:             preserve,
		BestEffortAttributes: f.archive,
		AttributesOnly:       f.attributesOnly,
		CopyContents:         f.copyContents,
		OneFileSystem:        f.oneFileSystem,
		KeepDirectorySymlink: f.keepDirectorySymlink,
		Force:                f.force && !f.noClobber,
		RemoveDestination:    f.removeDestination,
		HardLink:             f.hardLink,
		SymbolicLink:         f.symbolicLink,
		Reflink:              reflinkMode,
		Sparse:               sparseMode,
		Backup:               backupMode,
		BackupSuffix:         f.backupSuffix,
		Update:               updateMode,
		ContextLabel:         f.contextLabel,
		PreCallback:          setupPreCallback(f, updateMode, errw, &reader),
		Debug:                nil,
	}
	if f.debug {
		options.Debug = out
	}
	if f.backup == "" {
		options.Backup = cp.BackupNone
	}
	options.PostCallback = func(src, dst string) {
		if f.verbose {
			fmt.Fprintf(out, "%q -> %q\n", src, dst)
		}
	}

	var errs []error
	for _, rawSource := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		rawSource = sourcePathArgument(rawSource, f.stripTrailingSlashes)
		src := rawSource
		if resolvePath != nil {
			src = resolvePath(rawSource)
		}
		dst := resolvedDest
		if f.parents {
			dst = filepath.Join(resolvedDest, parentRelativePath(rawSource))
			if err := os.MkdirAll(filepath.Dir(dst), 0o777); err != nil {
				errs = append(errs, fmt.Errorf("cannot create parent directory for %q: %w", dst, err))
				continue
			}
		} else if destIsDir {
			dst = filepath.Join(resolvedDest, filepath.Base(rawSource))
		}

		srcInfo, statErr := os.Stat(src)
		if statErr == nil && f.noTargetDirectory && srcInfo.IsDir() && destWasDir {
			dstInfo, dstErr := os.Lstat(dst)
			keepSymlinkDir := f.keepDirectorySymlink && dstErr == nil && dstInfo.Mode()&os.ModeSymlink != 0
			if !keepSymlinkDir {
				errs = append(errs, fmt.Errorf("cannot overwrite directory %q with directory %q", dst, src))
				continue
			}
		}
		var copyErr error
		if f.recursive || f.archive {
			copyErr = options.CopyTree(src, dst)
		} else {
			copyErr = options.Copy(src, dst)
		}
		if copyErr != nil {
			errs = append(errs, fmt.Errorf("cannot copy %q to %q: %w", rawSource, dst, copyErr))
		}
	}
	return errors.Join(errs...)
}

func run(args []string, w io.Writer, input *bufio.Reader) error {
	return runWithCtx(context.Background(), args, w, w, input, nil)
}

// Main runs cp reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	out := ioEnv.Stdout
	if out == nil {
		out = io.Discard
	}
	errw := ioEnv.Stderr
	if errw == nil {
		errw = io.Discard
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(errw, "cp panic: %v\n", r)
			exitCode = 1
		}
	}()

	var input *bufio.Reader
	if ioEnv.Stdin != nil {
		input = bufio.NewReader(ioEnv.Stdin)
	}
	if err := runWithCtx(ctx, args, out, errw, input, ioEnv.ResolvePath); err != nil {
		fmt.Fprintf(errw, "cp: %v\n", err)
		return 1
	}
	return 0
}
