// SPDX-License-Identifier: MIT
// Copyright 2026 Google LLC
//
// Package ufs runs a 9p server.
//
// Start the server:
//
//	ufs :3333 -root /path/to/share
//
// Then, connect using the Linux 9P filesystem:
//
//go:build !wasip1

package ufs

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"

	"github.com/hugelgupf/p9/fsimpl/localfs"
	"github.com/hugelgupf/p9/p9"
	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/p9dev"
	"github.com/u-root/u-root/pkg/ssh9p"
	"github.com/u-root/u-root/pkg/sshstream"
	"golang.org/x/crypto/ssh"
)

type params struct {
	addr           string
	root           string
	useSSH         bool
	authorizedKeys string
	hostkey        string
	noDevMap       bool
}

func runWithIO(ctx context.Context, ioEnv core.IO, p params) error {
	var cfg *ssh.ServerConfig
	if p.useSSH {
		_cfg, err := sshstream.NewServerConfig(ioEnv.ResolvePath(p.authorizedKeys), ioEnv.ResolvePath(p.hostkey))
		if err != nil {
			return fmt.Errorf("cfg: %w", err)
		}
		cfg = _cfg
	}

	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", p.addr)
	if err != nil {
		return fmt.Errorf("bind: %w", err)
	}
	defer l.Close()

	var attacher p9.Attacher
	attacher = localfs.Attacher(ioEnv.ResolvePath(p.root))
	if p.noDevMap {
		attacher = p9dev.New(attacher)
	}

	server := ssh9p.NewServer(p9.NewServer(attacher), ssh9p.WithSSHServer(cfg))
	if err := server.Serve(ctx, l); err != nil && !errors.Is(err, net.ErrClosed) && ctx.Err() == nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// Main runs the ufs 9p file server reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "ufs: panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("ufs", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var p params
	fs.StringVar(&p.root, "root", "/", "Root of file system to serve")
	fs.BoolVar(&p.useSSH, "ssh", false, "Use ssh for transport")
	fs.StringVar(&p.authorizedKeys, "authorized_keys", "authorized_keys", "Path to the authorized_keys file")
	fs.StringVar(&p.hostkey, "hostkey", "id_rsa", "Path to the host's private key")
	fs.BoolVar(&p.noDevMap, "nodevmap", false, "Pretend special device files are regular files")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if fs.NArg() != 1 {
		fmt.Fprintf(ioEnv.Stderr, "Usage: ufs [options] <addr>\n\n")
		fs.Usage()
		return 1
	}
	p.addr = fs.Arg(0)

	if err := runWithIO(ctx, ioEnv, p); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "ufs: %v\n", err)
		return 1
	}
	return 0
}
