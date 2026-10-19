// Copyright 2018-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (!tinygo || tinygo.enable) && !wasip1

package sshd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mdlayher/vsock"
	"github.com/pkg/sftp"
	"github.com/u-root/u-root/pkg/core"
	"golang.org/x/crypto/ssh"
)

// The ssh package does not define these things so we will
type (
	ptyReq struct {
		TERM   string // TERM environment variable value (e.g., vt100)
		Col    uint32
		Row    uint32
		Xpixel uint32
		Ypixel uint32
		Modes  string // encoded terminal modes
	}
	execReq struct {
		Command string
	}
	exitStatusReq struct {
		ExitStatus uint32
	}
)

var dprintf = func(string, ...any) {}

// start a command
func runCommand(c ssh.Channel, p *ptySession, cmd string, args ...string) error {
	var ps *os.ProcessState
	var err error
	defer c.Close()

	if p != nil {
		ps, err = p.runCommand(c, cmd, args...)
		if err != nil {
			dprintf("Failed to execute PTY: %v", err)
			return err
		}
	} else {
		e := exec.Command(cmd, args...)
		e.Stdin, e.Stdout, e.Stderr = c, c, c
		log.Printf("Executing non-PTY command %s %v", cmd, args)
		// execute command and wait for response
		if err := e.Run(); err != nil {
			dprintf("Failed to execute: %v", err)
			return err
		}

		ps = e.ProcessState
	}

	if ps != nil && ps.Exited() {
		code := uint32(ps.ExitCode())
		dprintf("Exit status %v", code)
		c.SendRequest("exit-status", false, ssh.Marshal(exitStatusReq{code}))
	}
	return nil
}

func handleSftp(req *ssh.Request, channel ssh.Channel) error {
	workDir := "/"
	if cur, err := os.Getwd(); err == nil {
		workDir = cur
	}
	s, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(workDir))
	if err != nil {
		req.Reply(false, nil)
		return err
	}
	if err := s.Serve(); err != nil {
		req.Reply(false, nil)
		return err
	}

	req.Reply(true, nil)
	channel.CloseWrite() // EOF
	channel.SendRequest("exit-status", false, ssh.Marshal(exitStatusReq{0}))
	channel.Close()

	return nil
}

func session(chans <-chan ssh.NewChannel) {
	var p *ptySession
	// Service the incoming Channel channel.
	for newChannel := range chans {
		// Channels have a type, depending on the application level
		// protocol intended. In the case of a shell, the type is
		// "session" and ServerShell may be used to present a simple
		// terminal interface.
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Could not accept channel: %v", err)
			continue
		}

		// Sessions have out-of-band requests such as "shell",
		// "pty-req" and "env".
		go func(in <-chan *ssh.Request) {
			for req := range in {
				dprintf("Request %v", req.Type)
				switch req.Type {
				case "shell":
					req.Reply(true, nil)
					go runCommand(channel, p, defaultShell)
				case "exec":
					e := &execReq{}
					if err := ssh.Unmarshal(req.Payload, e); err != nil {
						log.Printf("sshd: %v", err)
						req.Reply(false, nil)
						break
					}
					req.Reply(true, nil)
					go runCommand(channel, p, defaultShell, defaultShellFlag, e.Command)
				case "pty-req":
					var err error
					p, err = newPTYSession(req.Payload)
					req.Reply(err == nil, nil)
				case "subsystem":
					switch {
					case len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp":
						err := handleSftp(req, channel)
						if err != nil {
							log.Printf("sshd: sftp: %v", err)
						}
					default:
						log.Printf("Not handling subsystem req %v %q",
							req, string(req.Payload))
						req.Reply(false, nil)
					}
				default:
					log.Printf("Not handling req %v %q", req, string(req.Payload))
					req.Reply(false, nil)
				}
			}
		}(requests)
	}
}

type params struct {
	keys    string
	privkey string
	net     string
	ip      string
	port    string
	debug   bool
}

func defaultParams() params {
	return params{
		debug:   false,
		keys:    "authorized_keys",
		privkey: "id_rsa",
		net:     "tcp",
		ip:      "0.0.0.0",
		port:    "2022",
	}
}

func parseParams() params {
	return defaultParams()
}

func listen(network, ip, port string) (net.Listener, error) {
	switch network {
	case "vsock":
		p, err := strconv.ParseUint(port, 0, 32)
		if err != nil {
			return nil, err
		}
		return vsock.ListenContextID(math.MaxUint32, uint32(p), nil)
	case "unix", "unixgram", "unixpacket":
		return net.Listen(network, port)
	case "":
		network = "tcp"
	}
	return net.Listen(network, net.JoinHostPort(ip, port))
}

type cmd struct {
	params
}

func command(p params) *cmd {
	return &cmd{
		params: p,
	}
}

func (c *cmd) run() error {
	return c.runWithCtx(context.Background(), core.IO{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
}

func (c *cmd) runWithCtx(ctx context.Context, ioEnv core.IO) error {
	if c.debug {
		dprintf = log.Printf
	}
	// Public key authentication is done by comparing
	// the public key of a received connection
	// with the entries in the authorized_keys file.
	authorizedKeysBytes, err := os.ReadFile(c.keys)
	if err != nil {
		return err
	}

	authorizedKeysMap := map[string]bool{}
	for len(authorizedKeysBytes) > 0 {
		pubKey, _, _, rest, err := ssh.ParseAuthorizedKey(authorizedKeysBytes)
		if err != nil {
			return err
		}

		authorizedKeysMap[string(pubKey.Marshal())] = true
		authorizedKeysBytes = rest
	}

	// An SSH server is represented by a ServerConfig, which holds
	// certificate details and handles authentication of ServerConns.
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
			if authorizedKeysMap[string(pubKey.Marshal())] {
				return &ssh.Permissions{
					Extensions: map[string]string{
						"pubkey-fp": ssh.FingerprintSHA256(pubKey),
					},
				}, nil
			}
			return nil, fmt.Errorf("unknown public key for %q", c.User())
		},
	}

	privateBytes, err := os.ReadFile(c.privkey)
	if err != nil {
		return err
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return err
	}

	config.AddHostKey(private)

	// Once a ServerConfig has been configured, connections can be accepted.
	listener, err := listen(c.net, c.ip, c.port)
	if err != nil {
		return err
	}
	defer listener.Close()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-stop:
		}
	}()

	for {
		nConn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			log.Printf("failed to accept incoming connection: %s", err)
			return err
		}

		// Before use, a handshake must be performed on the incoming net.Conn.
		conn, chans, reqs, err := ssh.NewServerConn(nConn, config)
		if err != nil {
			log.Printf("failed to handshake: %v", err)
			continue
		}
		log.Printf("%v logged in with key %s", conn.RemoteAddr(), conn.Permissions.Extensions["pubkey-fp"])

		// The incoming Request channel must be serviced.
		go ssh.DiscardRequests(reqs)

		go session(chans)
	}
}

// Main runs sshd with the given context, I/O environment, and arguments.
func Main(ctx context.Context, ioEnv core.IO, args ...string) int {
	if err := runWithIO(ctx, ioEnv, args...); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(ioEnv.Stderr, "sshd: %v\n", err)
		return 1
	}
	return 0
}

func runWithIO(ctx context.Context, ioEnv core.IO, args ...string) error {
	fs := flag.NewFlagSet("sshd", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	p := defaultParams()
	fs.BoolVar(&p.debug, "d", p.debug, "Enable debug prints")
	fs.StringVar(&p.keys, "keys", p.keys, "Path to the authorized_keys file")
	fs.StringVar(&p.privkey, "privatekey", p.privkey, "Path of private key")
	fs.StringVar(&p.net, "net", p.net, "network to listen on")
	fs.StringVar(&p.ip, "ip", p.ip, "ip address to listen on")
	fs.StringVar(&p.port, "port", p.port, "port to listen on")

	parseArgs := args
	if len(parseArgs) > 0 && !strings.HasPrefix(parseArgs[0], "-") {
		if parseArgs[0] == "sshd" || filepath.Base(parseArgs[0]) == "sshd" {
			parseArgs = parseArgs[1:]
		}
	}

	if err := fs.Parse(parseArgs); err != nil {
		return err
	}

	p.keys = ioEnv.ResolvePath(p.keys)
	p.privkey = ioEnv.ResolvePath(p.privkey)

	c := command(p)
	return c.runWithCtx(ctx, ioEnv)
}
