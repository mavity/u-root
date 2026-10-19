// SPDX-License-Identifier: MIT
// Copyright 2026 Google LLC
//
// Package ssh9p provides functions to serve and mount 9P servers over SSH.

package ssh9p

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/hugelgupf/p9/p9"
	"github.com/u-root/u-root/pkg/sshstream"
	"golang.org/x/crypto/ssh"
)

// Server wraps a p9 Server with optional SSH support.
type Server struct {
	*p9.Server
	cfg *ssh.ServerConfig
}

// ServerOpt is an option function.
type ServerOpt func(s *Server)

// WithSSHServer uses SSH for the 9P server's transport.
func WithSSHServer(cfg *ssh.ServerConfig) ServerOpt {
	return func(s *Server) {
		s.cfg = cfg
	}
}

// NewServer wraps a p9.Server.
func NewServer(p9s *p9.Server, opts ...ServerOpt) *Server {
	s := &Server{Server: p9s}

	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Serve handles requests from the listener.  Will close the listener when the
// context is done.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	stopf := context.AfterFunc(ctx, func() {
		l.Close()
	})
	defer stopf()

	for {
		c, err := l.Accept()
		if err != nil {
			return fmt.Errorf("accept: %w", err)
		}

		wg.Go(func() {
			// The func is a closure, so c will be whatever the
			// current version of c is, i.e. it could be nc from
			// below.
			stopf := context.AfterFunc(ctx, func() {
				c.Close()
			})
			defer func() {
				// Abort the AfterFunc (which might never run)
				// and close it ourselves.
				if stopf() {
					c.Close()
				}
			}()
			if s.cfg != nil {
				nc, err := sshstream.NewServer(c, s.cfg)
				if err != nil {
					log.Printf("ssh9p NewServer: %v", err)
					return

				}
				c = nc
			}
			if err := s.Handle(c, c); err != nil {
				log.Printf("ssh9p Handle: %v", err)
			}
		})
	}
}
