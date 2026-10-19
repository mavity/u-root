// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hostname prints or changes the system's hostname.
//
// Synopsis:
//
//	hostname [-s|-d|-f|-i] [-F file] [HOSTNAME]
package hostname

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var (
	errUsage = errors.New("usage: hostname [-s|-d|-f|-i] [-F file] [HOSTNAME]")
)

type flags struct {
	short  bool
	domain bool
	fqdn   bool
	ip     bool
	file   string
}

func getHostname(ioEnv core.IO) (string, error) {
	if h := ioEnv.Getenv("HOSTNAME"); h != "" {
		return h, nil
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h, nil
	}
	if c := ioEnv.Getenv("COMPUTERNAME"); c != "" {
		return c, nil
	}
	return "wasip1", nil
}

func setHostname(ctx context.Context, ioEnv core.IO, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("hostname cannot be empty")
	}

	_ = ioEnv.SetenvVal("HOSTNAME", name)

	if err := Sethostname(name); err != nil {
		return fmt.Errorf("could not set hostname: %w", err)
	}
	return nil
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	var f flags
	fs := flag.NewFlagSet("hostname", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	fs.BoolVar(&f.short, "s", false, "display short host name")
	fs.BoolVar(&f.short, "short", false, "display short host name")

	fs.BoolVar(&f.domain, "d", false, "display DNS domain name")
	fs.BoolVar(&f.domain, "domain", false, "display DNS domain name")

	fs.BoolVar(&f.fqdn, "f", false, "display fully qualified host name (FQDN)")
	fs.BoolVar(&f.fqdn, "fqdn", false, "display fully qualified host name (FQDN)")
	fs.BoolVar(&f.fqdn, "long", false, "display fully qualified host name (FQDN)")

	fs.BoolVar(&f.ip, "i", false, "display IP address for the host name")
	fs.BoolVar(&f.ip, "ip-address", false, "display IP address for the host name")

	fs.StringVar(&f.file, "F", "", "set host name from given file")
	fs.StringVar(&f.file, "file", "", "set host name from given file")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: hostname [-s|-d|-f|-i] [-F file] [HOSTNAME]\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	if f.file != "" {
		resolved := ioEnv.ResolvePath(f.file)
		data, err := os.ReadFile(resolved)
		if err != nil {
			return fmt.Errorf("could not read file %q: %w", f.file, err)
		}
		lines := strings.Split(string(data), "\n")
		name := ""
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "#") {
				name = l
				break
			}
		}
		return setHostname(ctx, ioEnv, name)
	}

	switch fs.NArg() {
	case 1:
		return setHostname(ctx, ioEnv, fs.Arg(0))
	case 0:
		h, err := getHostname(ioEnv)
		if err != nil {
			return fmt.Errorf("could not obtain hostname: %w", err)
		}

		out := h
		if f.short {
			out = strings.Split(h, ".")[0]
		} else if f.domain {
			if idx := strings.Index(h, "."); idx != -1 {
				out = h[idx+1:]
			} else {
				out = ""
			}
		} else if f.ip {
			addrs, err := net.LookupHost(h)
			if err == nil && len(addrs) > 0 {
				out = strings.Join(addrs, " ")
			} else {
				out = "127.0.0.1"
			}
		}

		fmt.Fprintln(ioEnv.Stdout, out)
		return nil
	default:
		return errUsage
	}
}

// Main runs the hostname command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "hostname: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "hostname: %v\n", err)
		}
		return 1
	}
	return 0
}
