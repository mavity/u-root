// Copyright 2016 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// created by Manoel Vilela <manoel_vilela@engineer.com>

package ln

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

type create struct {
	name string
	dir  bool
}

type result struct {
	symlink       bool
	name, linksTo string
}

type test struct {
	conf    config   // conf with flags
	args    []string // to pass for ln
	results []result // expected results
	files   []create // previous files for testing
	cmdline string   // cmdline ln equivalent
}

// loadTests loads the main table driven tests
func loadTests() []test {
	return []test{
		{
			config{},
			[]string{"a", "b"},
			[]result{{name: "b", linksTo: "a"}},
			[]create{{name: "a"}},
			"ln a b",
		},
		{
			config{symlink: true},
			[]string{"a", "b"},
			[]result{{symlink: true, name: "b", linksTo: "a"}},
			[]create{{name: "a"}},
			"ln -s a b",
		},
		{
			config{symlink: true},
			[]string{"bin/cp"},
			[]result{
				{symlink: true, name: "cp", linksTo: "bin/cp"},
			},
			[]create{
				{name: "bin", dir: true},
				{name: "bin/cp"},
			},
			"ln -s bin/cp",
		},
		{
			config{symlink: true},
			[]string{"bin/cp", "bin/ls", "bin/ln", "."},
			[]result{
				{symlink: true, name: "cp", linksTo: "bin/cp"},
				{symlink: true, name: "ls", linksTo: "bin/ls"},
				{symlink: true, name: "ln", linksTo: "bin/ln"},
			},
			[]create{
				{name: "bin", dir: true},
				{name: "bin/cp"},
				{name: "bin/ls"},
				{name: "bin/ln"},
			},
			"ln -s bin/cp bin/ls bin/ln .",
		},
		{
			config{symlink: true, dirtgt: "."},
			[]string{"bin/cp", "bin/ls", "bin/ln"},
			[]result{
				{symlink: true, name: "cp", linksTo: "bin/cp"},
				{symlink: true, name: "ls", linksTo: "bin/ls"},
				{symlink: true, name: "ln", linksTo: "bin/ln"},
			},
			[]create{
				{name: "bin", dir: true},
				{name: "bin/cp"},
				{name: "bin/ls"},
				{name: "bin/ln"},
			},
			"ln -s bin/cp bin/ls bin/ln -t .",
		},
		{
			config{symlink: true, dirtgt: "folder", relative: true},
			[]string{"cp", "ls", "ln"},
			[]result{
				{symlink: true, name: "folder/cp", linksTo: "../cp"},
				{symlink: true, name: "folder/ls", linksTo: "../ls"},
				{symlink: true, name: "folder/ln", linksTo: "../ln"},
			},
			[]create{
				{name: "folder", dir: true},
				{name: "cp"},
				{name: "ls"},
				{name: "ln"},
			},
			"ln -s -v -r -t folder cp ls ln",
		},
		{
			config{force: true, prompt: true},
			[]string{"a", "overwrite"},
			[]result{
				{name: "overwrite", linksTo: "a"},
			},
			[]create{
				{name: "overwrite"},
				{name: "a"},
			},
			"ln -i -f a overwrite",
		},
	}
}

func testHardLink(dir, linkName, target string, t *testing.T) {
	lp := filepath.Join(dir, linkName)
	tp := filepath.Join(dir, target)
	linkStat, err := os.Stat(lp)
	if err != nil {
		t.Errorf("stat %q failed: %v", lp, err)
		return
	}
	targetStat, err := os.Stat(tp)
	if err != nil {
		t.Errorf("stat %q failed: %v", tp, err)
		return
	}
	if !os.SameFile(linkStat, targetStat) {
		t.Errorf("link %q, %q did not create hard link", linkName, target)
	}
}

func testSymlink(dir, linkName, linksTo string, t *testing.T) {
	lp := filepath.Join(dir, linkName)
	linkStat, err := os.Stat(lp)
	if err != nil {
		t.Errorf("stat %q failed: %v", lp, err)
		return
	}

	tp := filepath.Join(filepath.Dir(lp), linksTo)
	targetStat, err := os.Stat(tp)
	if err != nil {
		t.Errorf("stat target %q failed: %v", tp, err)
		return
	}
	if !os.SameFile(linkStat, targetStat) {
		t.Errorf("symlink %q, %q did not resolve to same file", linkName, tp)
	}

	s, err := os.Readlink(lp)
	if err != nil {
		t.Errorf("readlink %q failed: %v", lp, err)
		return
	}

	// Normalize both slash styles for comparison
	expectedNorm := filepath.Clean(linksTo)
	actualNorm := filepath.Clean(s)
	if strings.ReplaceAll(actualNorm, "\\", "/") != strings.ReplaceAll(expectedNorm, "\\", "/") {
		t.Errorf("after symlink clean %q != %q (raw readlink=%q)", actualNorm, expectedNorm, s)
	}
}

func TestLn(t *testing.T) {
	tabDriven := loadTests()

	for caseNum, testCase := range tabDriven {
		d := t.TempDir()

		for _, f := range testCase.files {
			p := filepath.Join(d, f.name)
			if f.dir {
				if err := os.MkdirAll(p, 0o750); err != nil && err != os.ErrExist {
					t.Fatalf("Creation of dir %q fails: %v", p, err)
				}
			} else {
				if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte{'.'}, 0o640); err != nil {
					t.Fatal(err)
				}
			}
		}

		var cmdArgs []string
		if testCase.conf.symlink {
			cmdArgs = append(cmdArgs, "-s")
		}
		if testCase.conf.force {
			cmdArgs = append(cmdArgs, "-f")
		}
		if testCase.conf.prompt {
			cmdArgs = append(cmdArgs, "-i")
		}
		if testCase.conf.relative {
			cmdArgs = append(cmdArgs, "-r")
		}
		if testCase.conf.dirtgt != "" {
			cmdArgs = append(cmdArgs, "-t", testCase.conf.dirtgt)
		}
		cmdArgs = append(cmdArgs, testCase.args...)

		ioEnv := core.IO{
			Stdin:  strings.NewReader("y\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
			Dir:    d,
		}

		if err := run(context.Background(), ioEnv, cmdArgs...); err != nil {
			t.Errorf("Fails: test [%d]. %v", caseNum+1, err)
			continue
		}

		for _, expected := range testCase.results {
			if expected.symlink {
				testSymlink(d, expected.name, expected.linksTo, t)
			} else {
				testHardLink(d, expected.name, expected.linksTo, t)
			}
		}
	}
}
