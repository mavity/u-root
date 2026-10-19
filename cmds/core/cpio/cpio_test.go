// Copyright 2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpiocmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
)

type dirEnt struct {
	FileInfo os.FileInfo
	Name     string
	Type     string
	Content  string
	Target   string
}

// prepareTestDir creates a testing directory and returns a list of
// dirEnts and inputFile for archive creation
func prepareTestDir(t *testing.T, tempDir string) ([]dirEnt, *os.File) {
	t.Helper()
	targets := []dirEnt{
		{Name: "file1", Type: "file", Content: "Hello World"},
		{Name: "file2", Type: "file", Content: ""},
		{Name: "directory1", Type: "dir"},
	}
	if runtime.GOOS != "plan9" && runtime.GOOS != "windows" {
		targets = append(targets, []dirEnt{
			{Name: "hardlinked", Type: "hardlink", Target: "file1", Content: "Hello World"},
			{Name: "hardlinkedtofile2", Type: "hardlink", Target: "file2"},
		}...)
	}
	for _, ent := range targets {
		name := filepath.Join(tempDir, ent.Name)
		switch ent.Type {
		case "dir":
			err := os.Mkdir(name, os.FileMode(0o700))
			if err != nil {
				t.Fatalf("cannot create test directory: %v", err)
			}

		case "file":
			f, err := os.Create(name)
			if err != nil {
				t.Fatalf("cannot create test file: %v", err)
			}
			defer f.Close()
			_, err = f.WriteString(ent.Content)
			if err != nil {
				t.Fatal(err)
			}

		case "hardlink":
			target := filepath.Join(tempDir, ent.Target)
			err := os.Link(target, name)
			if err != nil {
				t.Fatalf("cannot create hard link: %v", err)
			}
		}
	}

	// Now that the temporary directory structure is complete, populate
	// the FileInfo for each target. This needs to happen in a second
	// pass because of the link count.
	for key, ent := range targets {
		var err error
		name := filepath.Join(tempDir, ent.Name)
		targets[key].FileInfo, err = os.Stat(name)
		if err != nil {
			t.Fatalf("cannot stat temporary dirent: %v", err)
		}
	}

	inputFile, err := os.CreateTemp(tempDir, "")
	if err != nil {
		t.Fatalf("%v", err)
	}

	for _, ent := range targets {
		name := ent.Name
		if _, err := fmt.Fprintln(inputFile, name); err != nil {
			t.Fatalf("failed to write file path %v to input file: %v", ent.Name, err)
		}
	}
	inputFile.Seek(0, 0)

	return targets, inputFile
}

func TestCpioList(t *testing.T) {
	tmpDir := t.TempDir()
	targets, inputFile := prepareTestDir(t, tmpDir)
	defer inputFile.Close()

	archive, err := os.CreateTemp(tmpDir, "archive.cpio")
	if err != nil {
		t.Fatalf("failed to create temporary archive file: %v", err)
	}
	defer archive.Close()

	ioEnv := core.IO{Dir: tmpDir, Stdin: inputFile, Stdout: archive}
	err = run(context.Background(), ioEnv, "o")
	if err != nil {
		t.Fatalf("failed to build archive from filepaths: %v", err)
	}

	if _, err := archive.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	ioEnv = core.IO{Dir: tmpDir, Stdin: archive, Stdout: stdout}
	err = run(context.Background(), ioEnv, "t")
	if err != nil {
		t.Fatalf("failed to list archive: %v", err)
	}

	stdoutStr := stdout.String()
	for _, ent := range targets {
		if !strings.Contains(stdoutStr, ent.Name) {
			t.Errorf("expected to find %q in output", ent.Name)
		}
	}
}

func TestCpio(t *testing.T) {
	tempDir := t.TempDir()
	targets, inputFile := prepareTestDir(t, tempDir)
	defer inputFile.Close()

	archive := &bytes.Buffer{}
	ioEnv := core.IO{Dir: tempDir, Stdin: inputFile, Stdout: archive}
	err := run(context.Background(), ioEnv, "o")
	if err != nil {
		t.Fatalf("failed to build archive from filepaths: %v", err)
	}

	// Extract to a new directory
	tempExtractDir := t.TempDir()

	out := &bytes.Buffer{}
	extractEnv := core.IO{Dir: tempExtractDir, Stdin: bytes.NewReader(archive.Bytes()), Stdout: out, Stderr: out}
	err = run(context.Background(), extractEnv, "-v", "i")
	if err != nil {
		t.Fatalf("Extraction failed:\n%#v\n%v\n", out.String(), err)
	}

	for _, ent := range targets {
		name := filepath.Join(tempExtractDir, ent.Name)

		newFileInfo, err := os.Stat(name)
		if err != nil {
			t.Error(err)
			continue
		}
		checkFileInfo(t, &ent, newFileInfo)

		if ent.Type != "dir" {
			content, err := os.ReadFile(name)
			if err != nil {
				t.Error(err)
			}
			if string(content) != ent.Content {
				t.Errorf("File %s has mismatched contents", ent.Name)
			}
		}
	}
}

func TestDirectoryHardLink(t *testing.T) {
	archiveFile, err := os.Open("testdata/dir-hard-link.cpio")
	if err != nil {
		t.Fatal(err)
	}
	defer archiveFile.Close()

	tempExtractDir := t.TempDir()
	want := &bytes.Buffer{}
	ioEnv := core.IO{Dir: tempExtractDir, Stdin: archiveFile, Stdout: want}
	err = run(context.Background(), ioEnv, "i")
	if err != nil {
		t.Fatalf("Extraction failed:\n%v\n%v\n", want, err)
	}
}
