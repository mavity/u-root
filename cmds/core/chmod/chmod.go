// Copyright 2016-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package chmod implements a GNU-compatible subset of chmod.
package chmod

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/u-root/u-root/pkg/core"
)

const usage = "chmod: chmod [OPTION]... MODE FILE..."

var errBadUsage = errors.New(usage)

type reportMode uint8

const (
	reportNone reportMode = iota
	reportChanges
	reportVerbose
)

type flags struct {
	recursive    bool
	quiet        bool
	preserveRoot bool
	reference    string
	report       reportMode
}

type commandError struct {
	err   error
	quiet bool
}

func (e *commandError) Error() string { return e.err.Error() }
func (e *commandError) Unwrap() error { return e.err }

type target struct {
	path    string
	display string
	info    fs.FileInfo
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	f, positional, err := parseArguments(args, ioEnv.Stdout)
	if err != nil {
		return quietCommandError(f, err)
	}
	if len(positional) == 0 {
		return quietCommandError(f, errBadUsage)
	}

	var expression modeExpression
	var referenceMode *uint16
	var paths []string
	if f.reference != "" {
		if len(positional) == 0 {
			return errBadUsage
		}
		refPath := ioEnv.ResolvePath(f.reference)
		info, err := os.Stat(refPath)
		if err != nil {
			return quietCommandError(f, fmt.Errorf("bad reference file %q: %w", f.reference, err))
		}
		mode := fileModeToPOSIX(info.Mode())
		referenceMode = &mode
		paths = positional
	} else {
		if len(positional) < 2 {
			return quietCommandError(f, errBadUsage)
		}
		expression, err = parseModeExpression(positional[0])
		if err != nil {
			return quietCommandError(f, err)
		}
		paths = positional[1:]
	}

	umask := uint16(0)
	if referenceMode == nil && expression.needsUmask() {
		current, err := currentGuestUmask()
		if err != nil {
			return quietCommandError(f, fmt.Errorf("read umask: %w", err))
		}
		umask = uint16(current & 0o777)
	}

	var operationErrors []error
	for _, name := range paths {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		path := ioEnv.ResolvePath(name)
		if f.recursive {
			operationErrors = append(operationErrors, applyRecursive(
				ctx, path, name, expression, referenceMode, umask, f,
				ioEnv.Stdout,
			)...)
			continue
		}

		if err := applyPath(path, name, nil, expression, referenceMode, umask, f, ioEnv.Stdout); err != nil {
			operationErrors = append(operationErrors, err)
		}
	}
	if len(operationErrors) > 0 {
		return &commandError{err: errors.Join(operationErrors...), quiet: f.quiet}
	}
	return nil
}

func quietCommandError(f flags, err error) error {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return err
	}
	return &commandError{err: err, quiet: f.quiet}
}

func parseArguments(args []string, stdout io.Writer) (flags, []string, error) {
	f := flags{preserveRoot: true}
	var positional []string
	endOptions := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !endOptions && arg == "--" {
			endOptions = true
			continue
		}
		if !endOptions {
			switch arg {
			case "-R", "--recursive":
				f.recursive = true
				continue
			case "-v", "--verbose":
				f.report = reportVerbose
				continue
			case "-c", "--changes":
				f.report = reportChanges
				continue
			case "-f", "--silent", "--quiet":
				f.quiet = true
				continue
			case "--preserve-root":
				f.preserveRoot = true
				continue
			case "--no-preserve-root":
				f.preserveRoot = false
				continue
			case "--help":
				fmt.Fprintln(stdout, usage)
				return f, nil, flag.ErrHelp
			case "--version":
				fmt.Fprintln(stdout, "chmod (u-root GNU-compatible subset)")
				return f, nil, flag.ErrHelp
			case "--reference":
				if i+1 >= len(args) {
					return f, nil, fmt.Errorf("flag needs an argument: %s", arg)
				}
				i++
				f.reference = args[i]
				if f.reference == "" {
					return f, nil, errors.New("--reference requires a file")
				}
				continue
			}
			if strings.HasPrefix(arg, "--reference=") {
				f.reference = strings.TrimPrefix(arg, "--reference=")
				if f.reference == "" {
					return f, nil, errors.New("--reference requires a file")
				}
				continue
			}
			if strings.HasPrefix(arg, "--") {
				return f, nil, fmt.Errorf("unknown option %q", arg)
			}
			if strings.HasPrefix(arg, "-") {
				if _, err := parseModeExpression(arg); err != nil {
					return f, nil, fmt.Errorf("unknown option or invalid mode %q", arg)
				}
			}
		}
		positional = append(positional, arg)
	}
	return f, positional, nil
}

func applyRecursive(
	ctx context.Context,
	operandPath, displayPath string,
	expression modeExpression,
	referenceMode *uint16,
	umask uint16,
	f flags,
	stdout interface{ Write([]byte) (int, error) },
) []error {
	resolvedPath, err := filepath.EvalSymlinks(operandPath)
	if err != nil {
		return []error{fmt.Errorf("%s: %w", displayPath, err)}
	}
	absolutePath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return []error{fmt.Errorf("%s: %w", displayPath, err)}
	}
	if f.preserveRoot && isRootPath(absolutePath) {
		return []error{fmt.Errorf("%s: refusing to operate recursively on root", displayPath)}
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		return []error{fmt.Errorf("%s: %w", displayPath, err)}
	}
	var targets []target
	var walkErrors []error
	collectTargets(ctx, absolutePath, displayPath, info, &targets, &walkErrors)
	for _, item := range targets {
		select {
		case <-ctx.Done():
			return append(walkErrors, ctx.Err())
		default:
		}
		if err := applyPath(item.path, item.display, item.info, expression, referenceMode, umask, f, stdout); err != nil {
			walkErrors = append(walkErrors, err)
		}
	}
	return walkErrors
}

func collectTargets(
	ctx context.Context,
	path, display string,
	info fs.FileInfo,
	targets *[]target,
	errs *[]error,
) {
	select {
	case <-ctx.Done():
		*errs = append(*errs, ctx.Err())
		return
	default:
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			*errs = append(*errs, fmt.Errorf("%s: %w", display, err))
		} else {
			for _, entry := range entries {
				childPath := filepath.Join(path, entry.Name())
				childDisplay := filepath.Join(display, entry.Name())
				childInfo, err := os.Lstat(childPath)
				if err != nil {
					*errs = append(*errs, fmt.Errorf("%s: %w", childDisplay, err))
					continue
				}
				if childInfo.Mode()&os.ModeSymlink != 0 {
					continue
				}
				collectTargets(ctx, childPath, childDisplay, childInfo, targets, errs)
			}
		}
	}
	*targets = append(*targets, target{path: path, display: display, info: info})
}

func applyPath(
	path, display string,
	info fs.FileInfo,
	expression modeExpression,
	referenceMode *uint16,
	umask uint16,
	f flags,
	stdout interface{ Write([]byte) (int, error) },
) error {
	if info == nil {
		var err error
		info, err = os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s: %w", display, err)
		}
	}

	oldMode := fileModeToPOSIX(info.Mode())
	newMode := oldMode
	maskConflict := false
	if referenceMode != nil {
		newMode = *referenceMode
	} else {
		newMode, maskConflict = expression.apply(oldMode, info.IsDir(), umask)
	}
	if err := os.Chmod(path, posixModeToFileMode(newMode)); err != nil {
		return fmt.Errorf("%s: %w", display, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: verify permissions: %w", display, err)
	}
	actualMode := fileModeToPOSIX(after.Mode())

	if f.report == reportVerbose || (f.report == reportChanges && oldMode != actualMode) {
		if oldMode == actualMode {
			fmt.Fprintf(stdout, "mode of %q retained as %04o (%s)\n", display, actualMode, permissionString(actualMode))
		} else {
			fmt.Fprintf(stdout, "mode of %q changed from %04o (%s) to %04o (%s)\n",
				display, oldMode, permissionString(oldMode), actualMode, permissionString(actualMode))
		}
	}
	if maskConflict {
		expected, _ := expression.apply(oldMode, info.IsDir(), 0)
		return fmt.Errorf("%s: new permissions are %s, not %s",
			display, permissionString(actualMode), permissionString(expected))
	}
	if actualMode != newMode {
		return fmt.Errorf("%s: requested mode %04o, but resulting mode is %04o",
			display, newMode, actualMode)
	}
	return nil
}

func fileModeToPOSIX(mode os.FileMode) uint16 {
	result := uint16(mode.Perm())
	if mode&os.ModeSetuid != 0 {
		result |= 0o4000
	}
	if mode&os.ModeSetgid != 0 {
		result |= 0o2000
	}
	if mode&os.ModeSticky != 0 {
		result |= 0o1000
	}
	return result
}

func posixModeToFileMode(mode uint16) os.FileMode {
	result := os.FileMode(mode & 0o777)
	if mode&0o4000 != 0 {
		result |= os.ModeSetuid
	}
	if mode&0o2000 != 0 {
		result |= os.ModeSetgid
	}
	if mode&0o1000 != 0 {
		result |= os.ModeSticky
	}
	return result
}

func permissionString(mode uint16) string {
	var result [9]byte
	for i, bit := range []uint16{0o400, 0o200, 0o100, 0o040, 0o020, 0o010, 0o004, 0o002, 0o001} {
		switch i % 3 {
		case 0:
			result[i] = 'r'
		case 1:
			result[i] = 'w'
		default:
			result[i] = 'x'
		}
		if mode&bit == 0 {
			result[i] = '-'
		}
	}
	if mode&0o4000 != 0 {
		if mode&0o100 != 0 {
			result[2] = 's'
		} else {
			result[2] = 'S'
		}
	}
	if mode&0o2000 != 0 {
		if mode&0o010 != 0 {
			result[5] = 's'
		} else {
			result[5] = 'S'
		}
	}
	if mode&0o1000 != 0 {
		if mode&0o001 != 0 {
			result[8] = 't'
		} else {
			result[8] = 'T'
		}
	}
	return string(result[:])
}

func isRootPath(path string) bool {
	volume := filepath.VolumeName(path)
	root := volume + string(os.PathSeparator)
	return filepath.Clean(path) == filepath.Clean(root)
}

func Main(ctx context.Context, ioEnv core.IO, args ...string) int {
	err := run(ctx, ioEnv, args...)
	if err == nil {
		return 0
	}

	var commandErr *commandError
	if errors.As(err, &commandErr) {
		if !commandErr.quiet {
			fmt.Fprintf(ioEnv.Stderr, "chmod: %v\n", commandErr.err)
		}
		return 1
	}
	if !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintf(ioEnv.Stderr, "chmod: %v\n", err)
	} else {
		return 0
	}
	return 1
}

func (expr modeExpression) needsUmask() bool {
	for _, clause := range expr.clauses {
		if clause.whoOmitted {
			return true
		}
	}
	return false
}
