// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux

package ip

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"strings"

	"github.com/u-root/u-root/pkg/core"
)

const ipHelp = `Usage: ip [ OPTIONS ] OBJECT { COMMAND | help }
where  OBJECT := { address | help | link | neighbor | neighbour | route }
       OPTIONS := { -4 | -6 | -br[ief] | -h[elp] }`

// Main executes ip command reentrantly on non-Linux platforms.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "ip: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("ip", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var (
		inet4  = fs.Bool("4", false, "IPv4 only")
		inet6  = fs.Bool("6", false, "IPv6 only")
		brief  = fs.Bool("br", false, "brief output")
		brief2 = fs.Bool("brief", false, "brief output")
	)

	fs.Usage = func() {
		fmt.Fprintf(ioEnv.Stderr, "%s\n", ipHelp)
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintf(ioEnv.Stdout, "%s\n", ipHelp)
		return 0
	}

	sub := rest[0]
	isBrief := *brief || *brief2

	switch {
	case strings.HasPrefix("help", sub):
		fmt.Fprintf(ioEnv.Stdout, "%s\n", ipHelp)
		return 0

	case strings.HasPrefix("address", sub) || sub == "a":
		return showAddresses(ctx, ioEnv, rest[1:], *inet4, *inet6, isBrief)

	case strings.HasPrefix("link", sub) || sub == "l":
		return showLinks(ctx, ioEnv, rest[1:], isBrief)

	case strings.HasPrefix("route", sub) || sub == "r":
		return showRoutes(ctx, ioEnv, rest[1:], *inet4, *inet6)

	case strings.HasPrefix("neighbor", sub) || strings.HasPrefix("neighbour", sub) || sub == "neigh":
		return showNeighbors(ctx, ioEnv)

	default:
		fmt.Fprintf(ioEnv.Stderr, "ip: object %q not supported or recognized\n", sub)
		return 1
	}
}

func showLinks(ctx context.Context, ioEnv core.IO, args []string, brief bool) int {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "ip link: %v\n", err)
		return 1
	}

	filterName := ""
	if len(args) > 0 {
		if args[0] == "show" || args[0] == "sh" || args[0] == "list" || args[0] == "ls" {
			if len(args) > 1 {
				filterName = args[1]
			}
		} else {
			filterName = args[0]
		}
	}

	for _, iface := range ifaces {
		select {
		case <-ctx.Done():
			return 130
		default:
		}

		if filterName != "" && iface.Name != filterName {
			continue
		}

		flagsStr := strings.ToUpper(strings.ReplaceAll(iface.Flags.String(), "|", ","))
		state := "DOWN"
		if iface.Flags&net.FlagUp != 0 {
			state = "UP"
		}

		if brief {
			fmt.Fprintf(ioEnv.Stdout, "%-16s %-8s %s\n", iface.Name, state, iface.HardwareAddr)
		} else {
			fmt.Fprintf(ioEnv.Stdout, "%d: %s: <%s> mtu %d state %s\n", iface.Index, iface.Name, flagsStr, iface.MTU, state)
			if len(iface.HardwareAddr) > 0 {
				fmt.Fprintf(ioEnv.Stdout, "    link/ether %s\n", iface.HardwareAddr)
			} else {
				fmt.Fprintf(ioEnv.Stdout, "    link/none\n")
			}
		}
	}
	return 0
}

func showAddresses(ctx context.Context, ioEnv core.IO, args []string, v4Only, v6Only, brief bool) int {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "ip addr: %v\n", err)
		return 1
	}

	filterName := ""
	if len(args) > 0 {
		if args[0] == "show" || args[0] == "sh" || args[0] == "list" || args[0] == "ls" {
			if len(args) > 1 {
				filterName = args[1]
			}
		} else if args[0] == "add" || args[0] == "del" || args[0] == "flush" {
			fmt.Fprintf(ioEnv.Stderr, "ip addr: modifying addresses is not supported on this platform\n")
			return 1
		} else {
			filterName = args[0]
		}
	}

	for _, iface := range ifaces {
		select {
		case <-ctx.Done():
			return 130
		default:
		}

		if filterName != "" && iface.Name != filterName {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		flagsStr := strings.ToUpper(strings.ReplaceAll(iface.Flags.String(), "|", ","))
		state := "DOWN"
		if iface.Flags&net.FlagUp != 0 {
			state = "UP"
		}

		if brief {
			var ipStrs []string
			for _, a := range addrs {
				ipNet, ok := a.(*net.IPNet)
				if !ok {
					continue
				}
				isV4 := ipNet.IP.To4() != nil
				if v4Only && !isV4 {
					continue
				}
				if v6Only && isV4 {
					continue
				}
				ipStrs = append(ipStrs, ipNet.String())
			}
			fmt.Fprintf(ioEnv.Stdout, "%-16s %-8s %s\n", iface.Name, state, strings.Join(ipStrs, " "))
		} else {
			fmt.Fprintf(ioEnv.Stdout, "%d: %s: <%s> mtu %d state %s\n", iface.Index, iface.Name, flagsStr, iface.MTU, state)
			if len(iface.HardwareAddr) > 0 {
				fmt.Fprintf(ioEnv.Stdout, "    link/ether %s\n", iface.HardwareAddr)
			} else {
				fmt.Fprintf(ioEnv.Stdout, "    link/none\n")
			}

			for _, a := range addrs {
				ipNet, ok := a.(*net.IPNet)
				if !ok {
					continue
				}
				isV4 := ipNet.IP.To4() != nil
				if v4Only && !isV4 {
					continue
				}
				if v6Only && isV4 {
					continue
				}

				if isV4 {
					fmt.Fprintf(ioEnv.Stdout, "    inet %s scope global %s\n", ipNet.String(), iface.Name)
				} else {
					fmt.Fprintf(ioEnv.Stdout, "    inet6 %s scope global %s\n", ipNet.String(), iface.Name)
				}
			}
		}
	}
	return 0
}

func showRoutes(ctx context.Context, ioEnv core.IO, args []string, v4Only, v6Only bool) int {
	if len(args) > 0 && (args[0] == "add" || args[0] == "del" || args[0] == "replace") {
		fmt.Fprintf(ioEnv.Stderr, "ip route: modifying routes is not supported on this platform\n")
		return 1
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "ip route: %v\n", err)
		return 1
	}

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
			if !ok {
				continue
			}
			isV4 := ipNet.IP.To4() != nil
			if v4Only && !isV4 {
				continue
			}
			if v6Only && isV4 {
				continue
			}
			mask := ipNet.Mask
			network := ipNet.IP.Mask(mask)
			fmt.Fprintf(ioEnv.Stdout, "%s/%d dev %s proto kernel scope link src %s\n",
				network.String(), ones(mask), iface.Name, ipNet.IP.String())
		}
	}
	return 0
}

func ones(mask net.IPMask) int {
	ones, _ := mask.Size()
	return ones
}

func showNeighbors(ctx context.Context, ioEnv core.IO) int {
	return 0
}
