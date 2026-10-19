// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package ping

import (
	"fmt"
	"net"

	"golang.org/x/net/icmp"
)

func listenPacket(net6 bool) (net.PacketConn, error) {
	netname, address := "ip4:icmp", "0.0.0.0"
	if net6 {
		netname, address = "ip6:ipv6-icmp", "::"
	}
	conn, err := icmp.ListenPacket(netname, address)
	if err == nil {
		return conn, nil
	}
	udpNet := "udp4"
	if net6 {
		udpNet = "udp6"
	}
	if uconn, uerr := icmp.ListenPacket(udpNet, address); uerr == nil {
		return uconn, nil
	}
	return nil, fmt.Errorf("can't setup %s socket on %s: %w", netname, address, err)
}
