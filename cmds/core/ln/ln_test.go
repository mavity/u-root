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
	"runtime"
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
			config{symlink: true, targetKind: "directory"},
			[]string{"folder", "folder-link"},
			[]result{{symlink: true, name: "folder-link", linksTo: "folder"}},
			[]create{{name: "folder", dir: true}},
			"ln -s --target-kind directory folder folder-link",
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
		if testCase.conf.targetKind != "" {
			cmdArgs = append(cmdArgs, "--target-kind", testCase.conf.targetKind)
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

func TestLnDanglingDirectorySymlinkTargetKind(t *testing.T) {
	dir := t.TempDir()
	ioEnv := core.IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard, Dir: dir}
	err := run(context.Background(), ioEnv, "-s", "--target-kind", "directory", "missing-directory", "link")
	if err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("host policy does not permit creating a dangling directory symlink: %v", err)
		}
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(dir, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "missing-directory" {
		t.Fatalf("stored link target = %q, want %q", target, "missing-directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "link")); !os.IsNotExist(err) {
		t.Fatalf("dangling directory link stat error = %v, want not-exist", err)
	}
}

func TestLnNoDereferenceDestinationDirectorySymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	actualDir := filepath.Join(dir, "actual-dir")
	linkToDir := filepath.Join(dir, "dir-link")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(actualDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actualDir, linkToDir); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("host policy does not permit creating a directory symlink: %v", err)
		}
		t.Fatal(err)
	}

	ioEnv := core.IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard, Dir: dir}
	if err := run(context.Background(), ioEnv, "-n", "-s", "-f", "target", "dir-link"); err != nil {
		t.Fatal(err)
	}
	storedTarget, err := os.Readlink(linkToDir)
	if err != nil {
		t.Fatal(err)
	}
	if storedTarget != "target" {
		t.Fatalf("destination symlink target = %q, want %q", storedTarget, "target")
	}
	if _, err := os.Lstat(filepath.Join(actualDir, "target")); !os.IsNotExist(err) {
		t.Fatalf("unexpected link was created inside destination directory: %v", err)
	}
}

func TestLnHardLinkSourceFollowModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this host does not expose a portable hard link to a symlink object")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target"), []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "source-link")); err != nil {
		t.Fatal(err)
	}
	ioEnv := core.IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard, Dir: dir}

	if err := run(context.Background(), ioEnv, "-P", "source-link", "physical-link"); err != nil {
		t.Fatal(err)
	}
	sourceLinkInfo, err := os.Lstat(filepath.Join(dir, "source-link"))
	if err != nil {
		t.Fatal(err)
	}
	physicalInfo, err := os.Lstat(filepath.Join(dir, "physical-link"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(sourceLinkInfo, physicalInfo) {
		t.Fatal("-P did not create a hard link to the source symlink itself")
	}

	if err := run(context.Background(), ioEnv, "-L", "source-link", "logical-link"); err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(filepath.Join(dir, "target"))
	if err != nil {
		t.Fatal(err)
	}
	logicalInfo, err := os.Stat(filepath.Join(dir, "logical-link"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(targetInfo, logicalInfo) {
		t.Fatal("-L did not create a hard link to the source symlink's referent")
	}
}
