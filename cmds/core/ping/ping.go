// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//go:build !tinygo || tinygo.enable

// Send icmp packets to a server to test network connectivity.
//
// Synopsis:
//
//	ping [-hV] [-c COUNT] [-i INTERVAL] [-s PACKETSIZE] [-w DEADLINE] DESTINATION
//
// Options:
//
//	-6: use ipv6 (ip6:ipv6-icmp)
//	-s: data size (default: 64)
//	-c: # iterations, 0 to run forever (default)
//	-i: interval in milliseconds (default: 1000)
//	-V: version
//	-w: wait time in milliseconds (default: 1000)
//	-a: Audible rings a bell when a packet is received
//	-h: help
//
// Description:
//
// This is a simplified implementation compared to iputils-ping.
// Notable differences:
//   - The -w flag sets a per-packet response timeout, not an overall deadline.
//   - No summary statistics are printed at the end.
//   - Exits with an error if a packet times out.
package ping

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"time"

	"github.com/u-root/u-root/pkg/core"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const usage = "ping [-V] [-6] [-c count] [-i interval] [-s packetsize] [-w deadline] [-a audible] destination"

type params struct {
	packetSize int
	intv       int
	wtf        int
	iter       uint64
	host       string
	net6       bool
	audible    bool
}

type cmd struct {
	stdout io.Writer
	conn   net.PacketConn
	params
}

func command(stdout io.Writer, p params) (*cmd, error) {
	conn, err := listenPacket(p.net6)
	if err != nil {
		return nil, err
	}
	return &cmd{stdout, conn, p}, nil
}

func (c *cmd) run() error {
	return c.runCtx(context.Background())
}

func (c *cmd) runCtx(ctx context.Context) error {
	defer c.conn.Close()
	if c.packetSize < 8 {
		return fmt.Errorf("packet size too small (must be >= 8): %v", c.packetSize)
	}

	network := "ip4"
	if c.net6 {
		network = "ip6"
	}

	addr, err := net.ResolveIPAddr(network, c.host)
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	interval := time.Duration(c.intv) * time.Millisecond
	waitFor := time.Duration(c.wtf) * time.Millisecond
	for i := uint64(0); i < c.iter; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := c.ping(addr, i+1, waitFor)
		if err != nil {
			return fmt.Errorf("ping failed: %w", err)
		}
		if c.audible {
			msg = "\a" + msg
		}
		fmt.Fprintf(c.stdout, "%s\n", msg)

		if i+1 < c.iter {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}
	}

	return nil
}

func (c *cmd) ping(addr *net.IPAddr, i uint64, waitFor time.Duration) (string, error) {
	c.conn.SetDeadline(time.Now().Add(waitFor))

	var echoRequestType icmp.Type = ipv4.ICMPTypeEcho
	if c.net6 {
		echoRequestType = ipv6.ICMPTypeEchoRequest
	}

	wm := icmp.Message{
		Type: echoRequestType, Code: 0, Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  int(i),
			Data: bytes.Repeat([]byte{1}, c.packetSize),
		},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return "", fmt.Errorf("icmp.Message.Marshal failed: %w", err)
	}

	startTime := time.Now()
	_, err = c.conn.WriteTo(wb, addr)
	if err != nil {
		return "", fmt.Errorf("conn.Write failed: %w", err)
	}

	var echoReplyType icmp.Type = ipv4.ICMPTypeEchoReply
	if c.net6 {
		echoReplyType = ipv6.ICMPTypeEchoReply
	}

	rb := make([]byte, 1500)
	var n int
	var msg *icmp.Message
	for msg == nil || msg.Type != echoReplyType {
		n, _, err = c.conn.ReadFrom(rb)
		if err != nil {
			return "", fmt.Errorf("conn.Read failed: %w", err)
		}
		msg, err = icmp.ParseMessage(echoReplyType.Protocol(), rb[:n])
		if err != nil {
			return "", fmt.Errorf("icmp.ParseMessage failed: %w", err)
		}
	}
	latency := time.Since(startTime)

	echoReply, ok := msg.Body.(*icmp.Echo)
	if !ok {
		return "", fmt.Errorf("got %+v; want echo reply", msg)
	}

	if echoReply.ID != os.Getpid()&0xffff {
		return "", fmt.Errorf("got id %v; want %v", echoReply.ID, os.Getpid()&0xffff)
	}
	if echoReply.Seq != int(i) {
		return "", fmt.Errorf("got seq %v; want %v", echoReply.Seq, i)
	}

	return fmt.Sprintf("%d bytes from %v: icmp_seq=%v time=%v", n, c.host, i, latency), nil
}

// Main is the reentrant entry point for ping.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "ping: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var (
		net6       = fs.Bool("6", false, "use ipv4 (means ip4:icmp) or 6 (ip6:ipv6-icmp)")
		packetSize = fs.Int("s", 56, "Data size")
		iter       = fs.Uint64("c", math.MaxUint64, "# iterations")
		intv       = fs.Int("i", 1000, "interval in milliseconds")
		wtf        = fs.Int("w", 1000, "wait time in milliseconds")
		audible    = fs.Bool("a", false, "Audible rings a bell when a packet is received")
		version    = fs.Bool("V", false, "print version")
	)

	fs.Usage = func() {
		fmt.Fprintf(ioEnv.Stderr, "Usage: %s\n", usage)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *version {
		fmt.Fprintln(ioEnv.Stdout, "u-root ping")
		return 0
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return 1
	}

	host := fs.Arg(0)
	c, err := command(ioEnv.Stdout, params{*packetSize, *intv, *wtf, *iter, host, *net6, *audible})
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "ping: %v\n", err)
		return 1
	}

	if err := c.runCtx(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "ping: %v\n", err)
		return 1
	}
	return 0
}
