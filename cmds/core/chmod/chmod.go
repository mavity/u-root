// Copyright 2016-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// chmod changes mode bits (e.g. permissions) of a file.
//
// Synopsis:
//
//	chmod MODE FILE...
//
// Description:
//
//	MODE is a three character octal value or a string like a=rwx
package chmod

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/u-root/u-root/pkg/core"
)

const (
	special = 99999
	usage   = "chmod: chmod [mode] filepath"
)

var errBadUsage = errors.New(usage)

type flags struct {
	recursive bool
	reference string
}

func changeMode(path string, mode os.FileMode, octval uint64, mask uint64, operator string) error {
	// A special value for mask means the mode is fully described
	if mask == special {
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	currentMode := info.Mode()

	switch operator {
	case "+":
		mode = currentMode | os.FileMode(octval)
	case "-":
		mode = currentMode &^ os.FileMode(octval)
	case "=":
		mode = (currentMode & os.FileMode(mask)) | os.FileMode(octval)
	default:
		mode = (currentMode & os.FileMode(mask)) | os.FileMode(octval)
	}

	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	return nil
}

func calculateMode(modeString string) (mode os.FileMode, octval uint64, mask uint64, operator string, err error) {
	octval, err = strconv.ParseUint(modeString, 8, 32)
	if err == nil {
		if octval > 0o777 {
			return mode, octval, mask, operator, fmt.Errorf("%w: invalid octal value %0o. Value should be less than or equal to 0777", strconv.ErrRange, octval)
		}
		mask = special
		mode = os.FileMode(octval)
		operator = "="
		return
	}

	// Try with user/group specified first
	reMode := regexp.MustCompile("^([ugoa]+)([-+=])(.*)")
	m := reMode.FindStringSubmatch(modeString)

	// If no match, try without user/group (defaults to 'a' - all)
	if len(m) == 0 {
		reMode = regexp.MustCompile("^([-+=])(.*)")
		m = reMode.FindStringSubmatch(modeString)
		if len(m) > 0 {
			m = []string{m[0], "a", m[1], m[2]}
		}
	}

	reMode = regexp.MustCompile("^[rwx]*$")
	if len(m) < 4 || !reMode.MatchString(m[3]) {
		return mode, octval, mask, operator, fmt.Errorf("%w:unable to decode mode %q. Please use an octal value or a valid mode string", strconv.ErrSyntax, modeString)
	}

	var octvalDigit uint64
	if strings.Contains(m[3], "r") {
		octvalDigit += 4
	}
	if strings.Contains(m[3], "w") {
		octvalDigit += 2
	}
	if strings.Contains(m[3], "x") {
		octvalDigit++
	}

	operator = m[2]

	if strings.Contains(m[1], "o") || strings.Contains(m[1], "a") {
		octval += octvalDigit
	}
	if strings.Contains(m[1], "g") || strings.Contains(m[1], "a") {
		octval += octvalDigit << 3
	}
	if strings.Contains(m[1], "u") || strings.Contains(m[1], "a") {
		octval += octvalDigit << 6
	}

	if operator == "=" {
		mask = 0o777
		if strings.Contains(m[1], "o") || strings.Contains(m[1], "a") {
			mask = mask & 0o770
		}
		if strings.Contains(m[1], "g") || strings.Contains(m[1], "a") {
			mask = mask & 0o707
		}
		if strings.Contains(m[1], "u") || strings.Contains(m[1], "a") {
			mask = mask & 0o077
		}

		if strings.Contains(m[1], "a") {
			mask = special
			mode = os.FileMode(octval)
		}
	}

	return mode, octval, mask, operator, nil
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var f flags

	// Parse arguments manually to handle mode strings like -rwx, +x, etc.
	var parsedArgs []string
	var i int
	for i = 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if arg == "-recursive" || arg == "--recursive" || arg == "-R" {
			f.recursive = true
			continue
		}
		if arg == "-reference" || arg == "--reference" {
			if i+1 < len(args) {
				f.reference = args[i+1]
				i++
				continue
			}
			return fmt.Errorf("flag needs an argument: %s", arg)
		}
		if strings.HasPrefix(arg, "-reference=") || strings.HasPrefix(arg, "--reference=") {
			f.reference = strings.SplitN(arg, "=", 2)[1]
			continue
		}
		// If it starts with - but contains r, w, or x, treat as mode operand
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			if strings.ContainsAny(arg[1:], "rwx") || arg == "-" {
				parsedArgs = append(parsedArgs, arg)
				i++
				break
			}
		}
		parsedArgs = append(parsedArgs, arg)
		i++
		break
	}
	for ; i < len(args); i++ {
		parsedArgs = append(parsedArgs, args[i])
	}

	if len(parsedArgs) < 1 {
		return errBadUsage
	}

	if len(parsedArgs) < 2 && f.reference == "" {
		return errBadUsage
	}

	var (
		mode         os.FileMode
		octval, mask uint64
		operator     string
		fileList     []string
	)

	if f.reference != "" {
		refPath := ioEnv.ResolvePath(f.reference)
		fi, err := os.Stat(refPath)
		if err != nil {
			return fmt.Errorf("bad reference file: %w", err)
		}
		mask = special
		mode = fi.Mode()
		operator = "="
		fileList = parsedArgs
	} else {
		var err error
		if mode, octval, mask, operator, err = calculateMode(parsedArgs[0]); err != nil {
			return err
		}
		fileList = parsedArgs[1:]
	}

	var finalErr error

	for _, name := range fileList {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resolvedName := ioEnv.ResolvePath(name)
		if f.recursive {
			err := filepath.Walk(resolvedName, func(p string, _ os.FileInfo, err error) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				if err != nil {
					return err
				}
				return changeMode(p, mode, octval, mask, operator)
			})
			if err != nil {
				finalErr = err
				fmt.Fprintln(ioEnv.Stderr, err)
			}
		} else {
			err := changeMode(resolvedName, mode, octval, mask, operator)
			if err != nil {
				finalErr = err
				fmt.Fprintln(ioEnv.Stderr, err)
			}
		}
	}
	return finalErr
}

// Main runs the chmod command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "chmod: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "chmod: %v\n", err)
		}
		return 1
	}
	return 0
}
