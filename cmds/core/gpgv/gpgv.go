// Copyright 2016-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gpgv validates a signature against a file.
//
// Synopsis:
//
//	gpgv KEY SIG CONTENT
//
// Description:
//
//	It prints "OK\n" to stdout if the check succeeds and exits with 0. It
//	prints an error message and exits with non-0 otherwise.
package gpgv

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var (
	errUsage            = errors.New("usage: gpgv key sig content")
	errUnknownSignature = errors.New("unknown signature")
	errExpectedPacket   = errors.New("expected first packet to be PublicKey")
)

func readPublicSigningKey(keyf io.Reader) (*packet.PublicKey, error) {
	keypackets := packet.NewReader(keyf)
	p, err := keypackets.Next()
	if err != nil {
		return nil, err
	}

	pkt, ok := p.(*packet.PublicKey)
	if !ok {
		return nil, errExpectedPacket
	}
	return pkt, nil
}

func verifyDetachedSignature(key *packet.PublicKey, contentf, sigf io.Reader) error {
	packets := packet.NewReader(sigf)
	p, err := packets.Next()
	if err != nil {
		return err
	}

	sig, ok := p.(*packet.Signature)
	if !ok {
		return errUnknownSignature
	}

	h := sig.Hash.New()
	if _, err := io.Copy(h, contentf); err != nil && err != io.EOF {
		return err
	}

	return key.VerifySignature(h, sig)
}

func run(ctx context.Context, ioEnv core.IO, args ...string) error {
	fs := flag.NewFlagSet("gpgv", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: gpgv [options] key sig content\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 3 {
		return errUsage
	}

	keyfile := ioEnv.ResolvePath(rest[0])
	sigfile := ioEnv.ResolvePath(rest[1])
	datafile := ioEnv.ResolvePath(rest[2])

	keyf, err := os.Open(keyfile)
	if err != nil {
		return err
	}
	defer keyf.Close()

	sigf, err := os.Open(sigfile)
	if err != nil {
		return err
	}
	defer sigf.Close()

	contentf, err := os.Open(datafile)
	if err != nil {
		return err
	}
	defer contentf.Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	key, err := readPublicSigningKey(keyf)
	if err != nil {
		return fmt.Errorf("key: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err = verifyDetachedSignature(key, contentf, sigf); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	fmt.Fprintln(ioEnv.Stdout, "OK")
	return nil
}

// Main runs the gpgv command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "gpgv: panic: %v\n", r)
			exitCode = 1
		}
	}()

	if err := run(ctx, ioEnv, args...); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(ioEnv.Stderr, "gpgv: %v\n", err)
		}
		return 1
	}
	return 0
}
