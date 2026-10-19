// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package netstat

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"net"
	"strings"
	"unsafe"

	"github.com/u-root/u-root/pkg/core"
	"golang.org/x/sys/windows"
)

var (
	modiphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = modiphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = modiphlpapi.NewProc("GetExtendedUdpTable")
)

const (
	tcpTableOwnerPidAll = 5
	udpTableOwnerPid    = 1
	afInet              = 2
)

type mibTcpRowOwnerPid struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPid  uint32
}

type mibUdpRowOwnerPid struct {
	LocalAddr uint32
	LocalPort uint32
	OwningPid uint32
}

var tcpStates = map[uint32]string{
	1:  "CLOSED",
	2:  "LISTEN",
	3:  "SYN_SENT",
	4:  "SYN_RECV",
	5:  "ESTABLISHED",
	6:  "FIN_WAIT1",
	7:  "FIN_WAIT2",
	8:  "CLOSE_WAIT",
	9:  "CLOSING",
	10: "LAST_ACK",
	11: "TIME_WAIT",
	12: "DELETE_TCB",
}

const usage = `usage: netstat [-a | -l] [-t] [-u] [-r] [-i] [-n] [-h]`

// Main executes netstat on Windows.
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
		all        = fs.Bool("a", false, "display all sockets (default: connected)")
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

	if *route {
		return showRoutes(ioEnv)
	}

	if *interfaces {
		return showInterfaces(ioEnv)
	}

	_ = numeric
	showAll := *all
	showListen := *listening
	showTCP := *tcpOnly || (!*tcpOnly && !*udpOnly)
	showUDP := *udpOnly || (!*tcpOnly && !*udpOnly)

	fmt.Fprintf(ioEnv.Stdout, "Active Internet connections (servers and established)\n")
	fmt.Fprintf(ioEnv.Stdout, "%-6s %-22s %-22s %-12s %s\n", "Proto", "Local Address", "Foreign Address", "State", "PID")

	if showTCP {
		rows, err := getTCPRows()
		if err == nil {
			for _, r := range rows {
				select {
				case <-ctx.Done():
					return 130
				default:
				}

				stateStr := tcpStates[r.State]
				if stateStr == "" {
					stateStr = fmt.Sprintf("STATE_%d", r.State)
				}
				isListen := r.State == 2
				if !showAll && !showListen && isListen {
					continue
				}
				if showListen && !isListen && !showAll {
					continue
				}

				locIP := intToIP(r.LocalAddr)
				locPort := portToHost(r.LocalPort)
				remIP := intToIP(r.RemoteAddr)
				remPort := portToHost(r.RemotePort)

				loc := fmt.Sprintf("%s:%d", locIP, locPort)
				rem := fmt.Sprintf("%s:%d", remIP, remPort)
				if isListen {
					rem = "*:*"
				}

				fmt.Fprintf(ioEnv.Stdout, "%-6s %-22s %-22s %-12s %d\n",
					"tcp", loc, rem, stateStr, r.OwningPid)
			}
		}
	}

	if showUDP {
		rows, err := getUDPRows()
		if err == nil {
			for _, r := range rows {
				select {
				case <-ctx.Done():
					return 130
				default:
				}

				locIP := intToIP(r.LocalAddr)
				locPort := portToHost(r.LocalPort)
				loc := fmt.Sprintf("%s:%d", locIP, locPort)

				fmt.Fprintf(ioEnv.Stdout, "%-6s %-22s %-22s %-12s %d\n",
					"udp", loc, "*:*", "", r.OwningPid)
			}
		}
	}

	return 0
}

func getTCPRows() ([]mibTcpRowOwnerPid, error) {
	var size uint32
	procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 1, afInet, tcpTableOwnerPidAll, 0)
	if size == 0 {
		return nil, fmt.Errorf("GetExtendedTcpTable size 0")
	}

	buf := make([]byte, size)
	ret, _, err := procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		1,
		afInet,
		tcpTableOwnerPidAll,
		0,
	)
	if ret != 0 {
		return nil, err
	}

	numEntries := binary.LittleEndian.Uint32(buf[0:4])
	rowSize := int(unsafe.Sizeof(mibTcpRowOwnerPid{}))
	rows := make([]mibTcpRowOwnerPid, 0, numEntries)
	offset := 4
	for i := uint32(0); i < numEntries && offset+rowSize <= len(buf); i++ {
		row := *(*mibTcpRowOwnerPid)(unsafe.Pointer(&buf[offset]))
		rows = append(rows, row)
		offset += rowSize
	}
	return rows, nil
}

func getUDPRows() ([]mibUdpRowOwnerPid, error) {
	var size uint32
	procGetExtendedUdpTable.Call(0, uintptr(unsafe.Pointer(&size)), 1, afInet, udpTableOwnerPid, 0)
	if size == 0 {
		return nil, fmt.Errorf("GetExtendedUdpTable size 0")
	}

	buf := make([]byte, size)
	ret, _, err := procGetExtendedUdpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		1,
		afInet,
		udpTableOwnerPid,
		0,
	)
	if ret != 0 {
		return nil, err
	}

	numEntries := binary.LittleEndian.Uint32(buf[0:4])
	rowSize := int(unsafe.Sizeof(mibUdpRowOwnerPid{}))
	rows := make([]mibUdpRowOwnerPid, 0, numEntries)
	offset := 4
	for i := uint32(0); i < numEntries && offset+rowSize <= len(buf); i++ {
		row := *(*mibUdpRowOwnerPid)(unsafe.Pointer(&buf[offset]))
		rows = append(rows, row)
		offset += rowSize
	}
	return rows, nil
}

func intToIP(val uint32) net.IP {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], val)
	return net.IPv4(b[0], b[1], b[2], b[3])
}

func portToHost(port uint32) uint16 {
	return binary.BigEndian.Uint16([]byte{byte(port), byte(port >> 8)})
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
