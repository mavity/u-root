// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package ping

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/windows"
)

var (
	modiphlpapi        = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreateFile  = modiphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle = modiphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho    = modiphlpapi.NewProc("IcmpSendEcho")
)

type ipOptionInformation struct {
	Ttl         byte
	Tos         byte
	Flags       byte
	OptionsSize byte
	OptionsData uintptr
}

type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

type winIcmpConn struct {
	handle   windows.Handle
	deadline time.Time
	lastResp []byte
}

func listenPacket(net6 bool) (net.PacketConn, error) {
	netname, address := "ip4:icmp", "0.0.0.0"
	if net6 {
		netname, address = "ip6:ipv6-icmp", "::"
	}
	conn, err := icmp.ListenPacket(netname, address)
	if err == nil {
		return conn, nil
	}

	// On Windows, raw sockets require Administrator privileges.
	// Fall back to Win32 IcmpCreateFile / IcmpSendEcho for IPv4 unprivileged ping.
	if !net6 {
		h, _, callErr := procIcmpCreateFile.Call()
		if windows.Handle(h) != windows.InvalidHandle && h != 0 {
			return &winIcmpConn{handle: windows.Handle(h)}, nil
		}
		if callErr != nil && callErr != syscall.Errno(0) {
			err = callErr
		}
	}

	return nil, fmt.Errorf("can't setup %s socket on %s: %w", netname, address, err)
}

func (w *winIcmpConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if len(w.lastResp) == 0 {
		return 0, nil, fmt.Errorf("no response available")
	}
	n := copy(b, w.lastResp)
	w.lastResp = nil
	return n, nil, nil
}

func (w *winIcmpConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	m, err := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), b)
	if err != nil {
		return 0, fmt.Errorf("icmp.ParseMessage failed: %w", err)
	}
	echoReq, ok := m.Body.(*icmp.Echo)
	if !ok {
		return 0, fmt.Errorf("invalid ICMP body: %+v", m.Body)
	}

	var ip net.IP
	if ipAddr, ok := addr.(*net.IPAddr); ok {
		ip = ipAddr.IP.To4()
	} else {
		ip = net.ParseIP(addr.String()).To4()
	}
	if ip == nil {
		return 0, fmt.Errorf("windows ICMP fallback only supports IPv4 addresses: %v", addr)
	}

	dest := binary.LittleEndian.Uint32(ip)

	timeout := time.Second
	if !w.deadline.IsZero() {
		timeout = time.Until(w.deadline)
		if timeout <= 0 {
			timeout = 10 * time.Millisecond
		}
	}
	timeoutMs := uint32(timeout / time.Millisecond)
	if timeoutMs == 0 {
		timeoutMs = 1
	}

	data := echoReq.Data
	var dataPtr uintptr
	if len(data) > 0 {
		dataPtr = uintptr(unsafe.Pointer(&data[0]))
	}

	replySize := uint32(unsafe.Sizeof(icmpEchoReply{}) + uintptr(len(data)) + 8)
	if replySize < 128 {
		replySize = 128
	}
	replyBuf := make([]byte, replySize)

	ret, _, callErr := procIcmpSendEcho.Call(
		uintptr(w.handle),
		uintptr(dest),
		dataPtr,
		uintptr(len(data)),
		0,
		uintptr(unsafe.Pointer(&replyBuf[0])),
		uintptr(replySize),
		uintptr(timeoutMs),
	)
	if ret == 0 {
		return 0, fmt.Errorf("IcmpSendEcho failed: %w", callErr)
	}

	rep := (*icmpEchoReply)(unsafe.Pointer(&replyBuf[0]))
	if rep.Status != 0 {
		return 0, fmt.Errorf("ICMP echo returned status %d", rep.Status)
	}

	repMsg := icmp.Message{
		Type: ipv4.ICMPTypeEchoReply,
		Code: 0,
		Body: &icmp.Echo{
			ID:   echoReq.ID,
			Seq:  echoReq.Seq,
			Data: data,
		},
	}
	encoded, err := repMsg.Marshal(nil)
	if err != nil {
		return 0, err
	}
	w.lastResp = encoded
	return len(b), nil
}

func (w *winIcmpConn) Close() error {
	if w.handle != 0 && w.handle != windows.InvalidHandle {
		procIcmpCloseHandle.Call(uintptr(w.handle))
		w.handle = windows.InvalidHandle
	}
	return nil
}

func (w *winIcmpConn) LocalAddr() net.Addr {
	return nil
}

func (w *winIcmpConn) SetDeadline(t time.Time) error {
	w.deadline = t
	return nil
}

func (w *winIcmpConn) SetReadDeadline(t time.Time) error {
	w.deadline = t
	return nil
}

func (w *winIcmpConn) SetWriteDeadline(t time.Time) error {
	w.deadline = t
	return nil
}
