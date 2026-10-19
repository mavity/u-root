// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !windows

package netstat

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"strings"

	"github.com/u-root/u-root/pkg/core"
)

const usage = `usage: netstat [-a | -l] [-t] [-u] [-r] [-i] [-n] [-h]`

// Main executes netstat on other non-Linux, non-Windows platforms.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "netstat: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("netstat", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var (
		all        = fs.Bool("a", false, "display all sockets")
		listening  = fs.Bool("l", false, "display listening server sockets")
		tcpOnly    = fs.Bool("t", false, "display TCP sockets")
		udpOnly    = fs.Bool("u", false, "display UDP sockets")
		route      = fs.Bool("r", false, "display routing table")
		interfaces = fs.Bool("i", false, "display interface table")
		numeric    = fs.Bool("n", false, "numeric output")
		help       = fs.Bool("h", false, "help")
	)

	fs.Usage = func() {
		fmt.Fprintf(ioEnv.Stderr, "%s\n", usage)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *help {
		fs.Usage()
		return 0
	}

	_ = all
	_ = listening
	_ = tcpOnly
	_ = udpOnly
	_ = interfaces
	_ = numeric

	if *route {
		return showRoutes(ioEnv)
	}

	return showInterfaces(ioEnv)
}

func showInterfaces(ioEnv core.IO) int {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "netstat -i: %v\n", err)
		return 1
	}
	fmt.Fprintf(ioEnv.Stdout, "%-16s %-6s %-18s %s\n", "Iface", "MTU", "RX-OK", "Flags")
	for _, iface := range ifaces {
		flagsStr := strings.ToUpper(strings.ReplaceAll(iface.Flags.String(), "|", ","))
		fmt.Fprintf(ioEnv.Stdout, "%-16s %-6d %-18s %s\n", iface.Name, iface.MTU, "-", flagsStr)
	}
	return 0
}

func showRoutes(ioEnv core.IO) int {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "netstat -r: %v\n", err)
		return 1
	}
	fmt.Fprintf(ioEnv.Stdout, "%-16s %-16s %-16s %-6s %s\n", "Destination", "Gateway", "Genmask", "Flags", "Iface")
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			mask := ipNet.Mask
			network := ipNet.IP.Mask(mask)
			genmask := net.IPv4(mask[0], mask[1], mask[2], mask[3])
			fmt.Fprintf(ioEnv.Stdout, "%-16s %-16s %-16s %-6s %s\n",
				network.String(), "0.0.0.0", genmask.String(), "U", iface.Name)
		}
	}
	return 0
}
