// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package pci

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/u-root/u-root/pkg/core"
	"golang.org/x/sys/windows"
)

var locRegex = regexp.MustCompile(`PCI bus (\d+), device (\d+), function (\d+)`)
var venDevRegex = regexp.MustCompile(`(?i)VEN_([0-9A-F]{4})&DEV_([0-9A-F]{4})`)

type pciDeviceInfo struct {
	bus      int
	dev      int
	fun      int
	vendorID string
	deviceID string
	desc     string
}

const usage = "pci [-n] [-v] [-h]"

// Main executes pci on Windows by reading device information from HKLM\SYSTEM\CurrentControlSet\Enum\PCI.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "pci: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("pci", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	var (
		numbers = fs.Bool("n", false, "Show numeric IDs")
		verbose = fs.Bool("v", false, "verbose output")
		help    = fs.Bool("h", false, "show help")
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

	if *help {
		fs.Usage()
		return 0
	}

	_ = verbose

	devices, err := enumerateWindowsPCI()
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "pci: failed to enumerate PCI devices: %v\n", err)
		return 1
	}

	for _, d := range devices {
		select {
		case <-ctx.Done():
			return 130
		default:
		}

		slot := fmt.Sprintf("%02x:%02x.%d", d.bus, d.dev, d.fun)
		if *numbers {
			fmt.Fprintf(ioEnv.Stdout, "%s %s:%s\n", slot, strings.ToLower(d.vendorID), strings.ToLower(d.deviceID))
		} else {
			desc := d.desc
			if desc == "" {
				desc = "PCI device"
			}
			fmt.Fprintf(ioEnv.Stdout, "%s [%s:%s] %s\n", slot, strings.ToLower(d.vendorID), strings.ToLower(d.deviceID), desc)
		}
	}
	return 0
}

func regSubKeys(k windows.Handle) ([]string, error) {
	var names []string
	var index uint32
	buf := make([]uint16, 256)
	for {
		bufLen := uint32(len(buf))
		err := windows.RegEnumKeyEx(k, index, &buf[0], &bufLen, nil, nil, nil, nil)
		if err != nil {
			break
		}
		names = append(names, windows.UTF16ToString(buf[:bufLen]))
		index++
	}
	return names, nil
}

func regGetString(k windows.Handle, name string) (string, error) {
	var valType uint32
	var bufLen uint32
	namePtr := windows.StringToUTF16Ptr(name)
	err := windows.RegQueryValueEx(k, namePtr, nil, &valType, nil, &bufLen)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, bufLen/2+1)
	err = windows.RegQueryValueEx(k, namePtr, nil, &valType, (*byte)(unsafe.Pointer(&buf[0])), &bufLen)
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf), nil
}

func enumerateWindowsPCI() ([]pciDeviceInfo, error) {
	var rootKey windows.Handle
	err := windows.RegOpenKeyEx(windows.HKEY_LOCAL_MACHINE, windows.StringToUTF16Ptr(`SYSTEM\CurrentControlSet\Enum\PCI`), 0, windows.KEY_READ, &rootKey)
	if err != nil {
		return nil, err
	}
	defer windows.RegCloseKey(rootKey)

	deviceKeys, err := regSubKeys(rootKey)
	if err != nil {
		return nil, err
	}

	var devices []pciDeviceInfo
	for _, devKeyName := range deviceKeys {
		var ven, dev string
		if m := venDevRegex.FindStringSubmatch(devKeyName); len(m) == 3 {
			ven = m[1]
			dev = m[2]
		}

		var dk windows.Handle
		err := windows.RegOpenKeyEx(rootKey, windows.StringToUTF16Ptr(devKeyName), 0, windows.KEY_READ, &dk)
		if err != nil {
			continue
		}

		instanceKeys, err := regSubKeys(dk)
		if err != nil {
			windows.RegCloseKey(dk)
			continue
		}

		for _, instName := range instanceKeys {
			var ik windows.Handle
			err := windows.RegOpenKeyEx(dk, windows.StringToUTF16Ptr(instName), 0, windows.KEY_READ, &ik)
			if err != nil {
				continue
			}

			desc, _ := regGetString(ik, "DeviceDesc")
			if idx := strings.Index(desc, ";"); idx != -1 {
				desc = desc[idx+1:]
			}
			if friendly, _ := regGetString(ik, "FriendlyName"); friendly != "" {
				desc = friendly
			}

			busNum, devNum, funNum := 0, 0, 0
			loc, _ := regGetString(ik, "LocationInformation")
			if m := locRegex.FindStringSubmatch(loc); len(m) == 4 {
				busNum, _ = strconv.Atoi(m[1])
				devNum, _ = strconv.Atoi(m[2])
				funNum, _ = strconv.Atoi(m[3])
			}

			devices = append(devices, pciDeviceInfo{
				bus:      busNum,
				dev:      devNum,
				fun:      funNum,
				vendorID: ven,
				deviceID: dev,
				desc:     desc,
			})
			windows.RegCloseKey(ik)
		}
		windows.RegCloseKey(dk)
	}

	return devices, nil
}
