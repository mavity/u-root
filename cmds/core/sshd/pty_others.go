// Copyright 2018-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows || plan9 || wasip1 || (tinygo && !tinygo.enable)

package sshd

import (
	"errors"
	"os"

	"golang.org/x/crypto/ssh"
)

type ptySession struct{}

func newPTYSession(b []byte) (*ptySession, error) {
	return nil, errors.New("pty not supported on this platform")
}

func (s *ptySession) runCommand(c ssh.Channel, cmd string, args ...string) (*os.ProcessState, error) {
	return nil, errors.New("pty not supported on this platform")
}
