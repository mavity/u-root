// Copyright 2018-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (!tinygo || tinygo.enable) && !windows && !plan9 && !wasip1

package sshd

import (
	"io"
	"log"
	"os"

	"github.com/u-root/u-root/pkg/pty"
	"golang.org/x/crypto/ssh"
)

type ptySession struct {
	p *pty.Pty
}

func newPTYSession(b []byte) (*ptySession, error) {
	ptyReq := &ptyReq{}
	if err := ssh.Unmarshal(b, ptyReq); err != nil {
		return nil, err
	}
	p, err := pty.New()
	if err != nil {
		return nil, err
	}
	ws, err := p.TTY.GetWinSize()
	if err != nil {
		return nil, err
	}
	ws.Row = uint16(ptyReq.Row)
	ws.Ypixel = uint16(ptyReq.Ypixel)
	ws.Col = uint16(ptyReq.Col)
	ws.Xpixel = uint16(ptyReq.Xpixel)
	if err := p.TTY.SetWinSize(ws); err != nil {
		return nil, err
	}
	_ = os.Setenv("TERM", ptyReq.TERM)
	return &ptySession{p: p}, nil
}

func (s *ptySession) runCommand(c ssh.Channel, cmd string, args ...string) (*os.ProcessState, error) {
	log.Printf("Executing PTY command %s %v", cmd, args)
	s.p.Command(cmd, args...)
	if err := s.p.C.Start(); err != nil {
		return nil, err
	}
	defer s.p.C.Wait()
	go io.Copy(s.p.Ptm, c)
	go io.Copy(c, s.p.Ptm)
	return s.p.C.Process.Wait()
}
