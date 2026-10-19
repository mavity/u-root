// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix && !wasip1

package stty

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/termios"
)

func run(stdout, stderr io.Writer, ioEnv core.IO, args []string) error {
	fd := 0
	if f, ok := ioEnv.Stdin.(*os.File); ok {
		fd = int(f.Fd())
	}

	t, err := termios.GTTY(fd)
	if err != nil {
		return fmt.Errorf("termios.GTTY: %w", err)
	}

	cmdArgs := args
	if len(cmdArgs) == 0 {
		cmdArgs = []string{"pretty"}
	}

	switch cmdArgs[0] {
	case "pretty":
		fmt.Fprintf(stdout, "%v\n", t.String())
	case "dump":
		b, err := json.MarshalIndent(t, "", "\t")
		if err != nil {
			return fmt.Errorf("json marshal: %w", err)
		}
		fmt.Fprintf(stdout, "%s\n", b)
	case "load":
		if len(cmdArgs) != 2 {
			return errors.New("usage: stty load <file>")
		}
		path := ioEnv.ResolvePath(cmdArgs[1])
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("stty load: %w", err)
		}
		if err := json.Unmarshal(b, t); err != nil {
			return fmt.Errorf("stty load: %w", err)
		}
		n, err := t.STTY(fd)
		if err != nil {
			return fmt.Errorf("stty: %w", err)
		}
		fmt.Fprintf(stdout, "%v\n", n.String())
	case "raw":
		if _, err := termios.Raw(fd); err != nil {
			return fmt.Errorf("raw: %w", err)
		}
	default:
		if err := t.SetOpts(cmdArgs); err != nil {
			return fmt.Errorf("setting opts: %w", err)
		}
		n, err := t.STTY(fd)
		if err != nil {
			return fmt.Errorf("stty: %w", err)
		}
		fmt.Fprintf(stdout, "%v\n", n.String())
	}
	return nil
}
