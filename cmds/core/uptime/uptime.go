// Copyright 2013-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Get the time the machine has been up
// Synopsis:
//
//	uptime
package uptime

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var (
	errAverageFormat = errors.New("/proc/loadavg has less then 3 fields")
	errUptimeFormat  = errors.New("/proc/uptime is empty")
)

// loadavg takes in the contents of proc/loadavg, it then extracts and returns the three load averages as a string
func loadavg(contents string) (loadaverage string, err error) {
	loadavg := strings.Fields(contents)
	if len(loadavg) < 3 {
		return "", fmt.Errorf("%q: %w", contents, errAverageFormat)
	}
	return strings.Join(loadavg, ", "), nil
}

// uptime takes in the contents of proc/uptime it then extracts and returns the uptime in the format Days , Hours , Minutes ,Seconds
func uptime(contents string) (*time.Time, error) {
	uptimeArray := strings.Fields(contents)
	if len(uptimeArray) == 0 {
		return nil, errUptimeFormat
	}
	uptimeDuration, err := time.ParseDuration(string(uptimeArray[0]) + "s")
	if err != nil {
		return nil, err
	}
	uptime := time.Time{}.Add(uptimeDuration)
	return &uptime, nil
}

func formatUptime(now time.Time, d time.Duration, loadAverage string) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf(" %s up %d days, %d hours, %d min, loadaverage: %s\n",
		now.Format("15:04:05"), days, hours, mins, loadAverage)
}

func run(stdout io.Writer, uptimePath, loadavgPath string) error {
	procUptimeOutput, err := os.ReadFile(uptimePath)
	if err != nil {
		return err
	}

	uptimeTime, err := uptime(string(procUptimeOutput))
	if err != nil {
		return err
	}

	procLoadAvgOutput, err := os.ReadFile(loadavgPath)
	if err != nil {
		return err
	}
	loadAverage, err := loadavg(string(procLoadAvgOutput))
	if err != nil {
		return err
	}

	if stdout != nil {
		fmt.Fprintf(stdout, " %s up %d days, %d hours, %d min, loadaverage: %s\n",
			time.Now().Format("15:04:05"), (uptimeTime.Day() - 1), uptimeTime.Hour(), uptimeTime.Minute(), loadAverage)
	}
	return nil
}

// Main runs the uptime command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "uptime: panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("uptime", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	d, loadAvg, err := getSystemUptimeAndLoad()
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "uptime: %v\n", err)
		return 1
	}

	fmt.Fprint(ioEnv.Stdout, formatUptime(time.Now(), d, loadAvg))
	return 0
}
