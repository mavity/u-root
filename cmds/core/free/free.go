// Copyright 2012-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// free reports usage information for physical memory and swap space.
//
// Synopsis:
//
//	free [-k] [-m] [-g] [-t] [-h] [-b] [-json]
package free

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/u-root/u-root/pkg/core"
)

type unit uint

const (
	B  unit = 0
	KB      = 10
	MB      = 20
	GB      = 30
	TB      = 40
)

var units = [...]string{"B", "K", "M", "G", "T"}

var errMultipleUnits = fmt.Errorf("multiple unit options doesn't make sense")

// the following types are used for JSON serialization
type mainMemInfo struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Free      uint64 `json:"free"`
	Shared    uint64 `json:"shared"`
	Cached    uint64 `json:"cached"`
	Buffers   uint64 `json:"buffers"`
	Available uint64 `json:"available"`
}

type swapInfo struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
	Free  uint64 `json:"free"`
}

// MemInfo represents the main memory and swap space information in a structured
// manner, suitable for JSON encoding.
type MemInfo struct {
	Mem  mainMemInfo `json:"mem"`
	Swap swapInfo    `json:"swap"`
}

type cmd struct {
	stdout io.Writer
	unit   unit
	human  bool
	toJSON bool
}

type options struct {
	human  bool
	bytes  bool
	kbytes bool
	mbytes bool
	gbytes bool
	tbytes bool
	json   bool
}

func countTrue(b ...bool) int {
	var cnt int
	for _, v := range b {
		if v {
			cnt++
		}
	}
	return cnt
}

func command(stdout io.Writer, o options) (*cmd, error) {
	count := countTrue(o.bytes, o.kbytes, o.mbytes, o.gbytes, o.tbytes, o.human)
	if count > 1 {
		return nil, errMultipleUnits
	}

	c := &cmd{
		stdout: stdout,
		toJSON: o.json,
	}

	if o.human {
		c.human = true
	} else {
		switch {
		case o.bytes:
			c.unit = B
		case o.mbytes:
			c.unit = MB
		case o.gbytes:
			c.unit = GB
		case o.tbytes:
			c.unit = TB
		default:
			c.unit = KB
		}
	}

	return c, nil
}

func (c *cmd) formatValueByConfig(value uint64) string {
	if c.human {
		return strings.ReplaceAll(humanize.IBytes(value), " ", "")
	}
	return fmt.Sprintf("%v", value>>c.unit)
}

func (c *cmd) parse(m meminfomap) error {
	mmi, err := getMainMemInfo(m)
	if err != nil {
		return err
	}
	si, err := getSwapInfo(m)
	if err != nil {
		return err
	}
	mi := MemInfo{Mem: *mmi, Swap: *si}
	return c.printMemInfo(&mi)
}

func (c *cmd) printMemInfo(mi *MemInfo) error {
	if c.toJSON {
		jsonData, err := json.Marshal(mi)
		if err != nil {
			return err
		}
		fmt.Fprintln(c.stdout, string(jsonData))
	} else {
		fmt.Fprintf(c.stdout, "              total        used        free      shared  buff/cache   available\n")
		fmt.Fprintf(c.stdout, "%-7s %11v %11v %11v %11v %11v %11v\n",
			"Mem:",
			c.formatValueByConfig(mi.Mem.Total),
			c.formatValueByConfig(mi.Mem.Used),
			c.formatValueByConfig(mi.Mem.Free),
			c.formatValueByConfig(mi.Mem.Shared),
			c.formatValueByConfig(mi.Mem.Buffers+mi.Mem.Cached),
			c.formatValueByConfig(mi.Mem.Available),
		)
		fmt.Fprintf(c.stdout, "%-7s %11v %11v %11v\n",
			"Swap:",
			c.formatValueByConfig(mi.Swap.Total),
			c.formatValueByConfig(mi.Swap.Used),
			c.formatValueByConfig(mi.Swap.Free),
		)
	}
	return nil
}

// Main runs the free command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "free: panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("free", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	humanOutput := fs.Bool("h", false, "Human output: show automatically the shortest three-digits unit")
	inBytes := fs.Bool("b", false, "Express the values in bytes")
	inKB := fs.Bool("k", false, "Express the values in kibibytes (default)")
	inMB := fs.Bool("m", false, "Express the values in mebibytes")
	inGB := fs.Bool("g", false, "Express the values in gibibytes")
	inTB := fs.Bool("t", false, "Express the values in tebibytes")
	toJSON := fs.Bool("json", false, "Use JSON for output")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	o := options{
		human:  *humanOutput,
		bytes:  *inBytes,
		kbytes: *inKB,
		mbytes: *inMB,
		gbytes: *inGB,
		tbytes: *inTB,
		json:   *toJSON,
	}

	c, err := command(ioEnv.Stdout, o)
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "free: %v\n", err)
		return 1
	}

	mi, err := getPlatformMemInfo()
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "free: %v\n", err)
		return 1
	}

	if err := c.printMemInfo(mi); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "free: %v\n", err)
		return 1
	}
	return 0
}
