// Copyright 2012-2021 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//go:build !tinygo || tinygo.enable

// Wget reads one file from a url and writes to stdout.
//
// Synopsis:
//
//	wget URL
//
// Description:
//
//	Returns a non-zero code on failure.
//
// Notes:
//
//	There are a few differences with GNU wget:
//	- Upon error, the return value is always 1.
//	- The protocol (http/https) is mandatory.
//
// Example:
//
//	wget -O google.txt http://google.com/
package wget

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/curl"
	"github.com/u-root/uio/uio"
)

var errEmptyURL = errors.New("empty url")

type cmd struct {
	url        string
	outputPath string
}

// flags parses wget flags
// wget is old school, and allows flags after the URL.
// This code does not process the -- flag specified in the
// man page, as the command itself does not seem to either.
func flags(args ...string) (string, string, error) {
	if len(args) == 0 {
		return "", "", errEmptyURL
	}

	f := flag.NewFlagSet("wget", flag.ContinueOnError)
	outPath := f.String("O", "", "output file")

	parseArgs := args
	if len(parseArgs) > 0 && !strings.HasPrefix(parseArgs[0], "-") {
		if parseArgs[0] == "wget" || parseArgs[0] == "" || filepath.Base(parseArgs[0]) == "wget" {
			parseArgs = parseArgs[1:]
		}
	}

	if err := f.Parse(parseArgs); err != nil {
		return "", "", err
	}

	if len(f.Args()) == 0 {
		return "", "", errEmptyURL
	}

	URL := f.Args()[0]

	// Now, it is allowed to have switches after the URL,
	// handle following flags
	if err := f.Parse(f.Args()[1:]); err != nil {
		return "", "", err
	}

	return *outPath, URL, nil
}

func command(args ...string) (*cmd, error) {
	outPath, URL, err := flags(args...)
	if err != nil {
		return nil, err
	}

	return &cmd{
		outputPath: outPath,
		url:        URL,
	}, nil
}

func (c *cmd) runWithIO(ctx context.Context, ioEnv core.IO) error {
	if c.url == "" {
		return errEmptyURL
	}

	parsedURL, err := url.Parse(c.url)
	if err != nil {
		return err
	}

	if c.outputPath == "" {
		c.outputPath = defaultOutputPath(parsedURL.Path)
	}

	schemes := curl.Schemes{
		"tftp": curl.DefaultTFTPClient,
		"http": curl.DefaultHTTPClient,

		// curl.DefaultSchemes doesn't support HTTPS by default.
		"https": curl.DefaultHTTPClient,
		"file":  &curl.LocalFileClient{},
	}

	reader, err := schemes.FetchWithoutCache(ctx, parsedURL)
	if err != nil {
		return fmt.Errorf("failed to download %v: %w", c.url, err)
	}

	if c.outputPath == "-" {
		out := ioEnv.Stdout
		if out == nil {
			out = os.Stdout
		}
		_, err := io.Copy(out, reader)
		return err
	}

	targetPath := ioEnv.ResolvePath(c.outputPath)
	if err := uio.ReadIntoFile(reader, targetPath); err != nil {
		return err
	}

	return nil
}

func (c *cmd) run() error {
	return c.runWithIO(context.Background(), core.IO{Stdout: os.Stdout, Stderr: os.Stderr})
}

// Main executes the wget command with the given context and virtualized IO.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "wget: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	c, err := command(args...)
	if err != nil {
		if errors.Is(err, errEmptyURL) {
			fmt.Fprintf(ioEnv.Stderr, "Usage: wget [ARGS] URL\n")
			return 2
		}
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(ioEnv.Stderr, "wget: %v\n", err)
		return 1
	}

	if err := c.runWithIO(ctx, ioEnv); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "wget: %v\n", err)
		return 1
	}
	return 0
}

func defaultOutputPath(urlPath string) string {
	if urlPath == "" || strings.HasSuffix(urlPath, "/") {
		return "index.html"
	}
	return path.Base(urlPath)
}
