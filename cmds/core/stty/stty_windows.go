// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package stty

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/u-root/u-root/pkg/core"
	"golang.org/x/sys/windows"
)

type winConsoleState struct {
	Rows   int             `json:"rows"`
	Cols   int             `json:"cols"`
	InMode uint32 `json:"in_mode"`
	OutMode uint32 `json:"out_mode"`
}

func (w winConsoleState) String() string {
	return fmt.Sprintf("rows:%d cols:%d in_mode:0x%x out_mode:0x%x", w.Rows, w.Cols, w.InMode, w.OutMode)
}

func getWinConsole() (winConsoleState, error) {
	var st winConsoleState
	hIn := windows.Handle(os.Stdin.Fd())
	_ = windows.GetConsoleMode(hIn, &st.InMode)

	hOut := windows.Handle(os.Stdout.Fd())
	_ = windows.GetConsoleMode(hOut, &st.OutMode)

	var csbi windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(hOut, &csbi); err == nil {
		st.Cols = int(csbi.Window.Right - csbi.Window.Left + 1)
		st.Rows = int(csbi.Window.Bottom - csbi.Window.Top + 1)
	} else {
		st.Cols = 80
		st.Rows = 24
	}
	return st, nil
}

func run(stdout, stderr io.Writer, ioEnv core.IO, args []string) error {
	st, err := getWinConsole()
	if err != nil {
		return err
	}

	cmdArgs := args
	if len(cmdArgs) == 0 {
		cmdArgs = []string{"pretty"}
	}

	switch cmdArgs[0] {
	case "pretty":
		fmt.Fprintf(stdout, "%v\n", st.String())
	case "dump":
		b, err := json.MarshalIndent(st, "", "\t")
		if err != nil {
			return fmt.Errorf("json marshal: %w", err)
		}
		fmt.Fprintf(stdout, "%s\n", b)
	case "size":
		fmt.Fprintf(stdout, "%d %d\n", st.Rows, st.Cols)
	default:
		// Accept settings or print current state
		fmt.Fprintf(stdout, "%v\n", st.String())
	}
	return nil
}
