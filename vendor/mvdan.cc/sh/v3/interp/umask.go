// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"fmt"
	"strconv"
	"strings"
)

func (r *Runner) runUmask(args []string) int {
	symbolic := false
	printCommand := false
	var mode string
	hasMode := false
	endOptions := false

	for _, arg := range args {
		if !endOptions && arg == "--" {
			endOptions = true
			continue
		}
		if !endOptions {
			switch arg {
			case "-S":
				symbolic = true
				continue
			case "-p":
				printCommand = true
				continue
			}
			if strings.HasPrefix(arg, "-") {
				r.errf("umask: invalid option %q\n", arg)
				return 2
			}
		}
		if hasMode {
			r.errf("umask: too many arguments\n")
			return 2
		}
		mode = arg
		hasMode = true
	}

	if !hasMode {
		mask, err := currentUmask()
		if err != nil {
			r.errf("umask: %v\n", err)
			return 2
		}
		if symbolic {
			if printCommand {
				r.outf("umask -S %s\n", symbolicUmask(mask))
			} else {
				r.outf("%s\n", symbolicUmask(mask))
			}
		} else {
			if printCommand {
				r.outf("umask %04o\n", mask)
			} else {
				r.outf("%04o\n", mask)
			}
		}
		return 0
	}
	if symbolic || printCommand {
		r.errf("umask: -S and -p cannot be used when setting the mask\n")
		return 2
	}

	mask, err := strconv.ParseUint(mode, 8, 12)
	if err != nil || mask > 0o777 {
		r.errf("umask: invalid mask %q\n", mode)
		return 2
	}
	if _, err := setUmask(int(mask)); err != nil {
		r.errf("umask: %v\n", err)
		return 2
	}
	return 0
}

func symbolicUmask(mask int) string {
	allowed := 0o777 &^ mask
	class := func(shift uint) string {
		bits := (allowed >> shift) & 0o7
		var permissions strings.Builder
		if bits&0o4 != 0 {
			permissions.WriteByte('r')
		}
		if bits&0o2 != 0 {
			permissions.WriteByte('w')
		}
		if bits&0o1 != 0 {
			permissions.WriteByte('x')
		}
		return permissions.String()
	}
	return fmt.Sprintf("u=%s,g=%s,o=%s", class(6), class(3), class(0))
}
