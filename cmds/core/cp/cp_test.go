// Copyright 2016 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// created by Manoel Vilela <manoel_vilela@engineer.com>

package cpcmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/cp"
	"github.com/u-root/uio/uio"
)

const (
	maxSizeFile = 1000
	maxDirDepth = 5
	maxFiles    = 5
)

// randomFile create a random file with random content
func randomFile(fpath, prefix string) (*os.File, error) {
	f, err := os.CreateTemp(fpath, prefix)
	if err != nil {
		return nil, err
	}
	// generate random content for files
	bytes := []byte{}
	for i := 0; i < rand.Intn(maxSizeFile); i++ {
		bytes = append(bytes, byte(i))
	}
	f.Write(bytes)

	return f, nil
}

// createFilesTree create a random files tree
func createFilesTree(root string, maxDepth, depth int) error {
	// create more one dir if don't achieve the maxDepth
	if depth < maxDepth {
		newDir, err := os.MkdirTemp(root, fmt.Sprintf("cpdir_%d_", depth))
		if err != nil {
			return err
		}

		if err = createFilesTree(newDir, maxDepth, depth+1); err != nil {
			return err
		}
	}
	// generate random files
	for i := range maxFiles {
		f, err := randomFile(root, fmt.Sprintf("cpfile_%d_", i))
		if err != nil {
			return err
		}
		f.Close()
	}

	return nil
}

func TestRunSimple(t *testing.T) {
	tmpDir := t.TempDir()
	file1, err := randomFile(tmpDir, "src-")
	if err != nil {
		t.Errorf("failed to create tmp dir: %q", err)
	}
	defer file1.Close()

	for _, tt := range []struct {
		name                           string
		args                           []string
		recursive, ask, force, verbose bool
		input                          string
		wantErr                        error
	}{
		{
			name: "NoFlags-Success-",
			args: []string{file1.Name(), filepath.Join(tmpDir, "destination")},
		},
		{
			name:  "AskYes-Success-",
			args:  []string{"-i", file1.Name(), filepath.Join(tmpDir, "destination")},
			ask:   true,
			input: "yes\n",
		},
		{
			name:    "AskYes-Fail1-",
			args:    []string{"-i", file1.Name(), filepath.Join(tmpDir, "destination")},
			ask:     true,
			input:   "yes",
			wantErr: io.EOF,
		},
		{
			name:    "AskYes-Fail2-",
			args:    []string{"-i", file1.Name(), filepath.Join(tmpDir, "destination")},
			ask:     true,
			input:   "no\n",
			wantErr: cp.ErrSkip,
		},
		{
			name:    "Verbose",
			args:    []string{"-v", file1.Name(), filepath.Join(tmpDir, "destination")},
			verbose: true,
			wantErr: cp.ErrSkip,
		},
		{
			name:    "SameFile-NoFlags",
			args:    []string{file1.Name(), file1.Name()},
			wantErr: errSameFile,
		},
		{
			name:    "NoFlags-Fail-SrcNotExist",
			args:    []string{"src", filepath.Join(tmpDir, "destination")},
			wantErr: fs.ErrNotExist,
		},
		{
			name:    "NoFlags-Fail-DstcNotExist",
			args:    []string{file1.Name(), "dst"},
			wantErr: fs.ErrNotExist,
		},
		{
			name:    "NoFlags-ToManyArgs-",
			args:    []string{file1.Name(), "dst", "src"},
			wantErr: eNotDir,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			var inBuf bytes.Buffer
			fmt.Fprintf(&inBuf, "%s", tt.input)
			in := bufio.NewReader(&inBuf)
			if err := run(tt.args, &out, in); err != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf(`run(tt.args, &out, in) = %q, not %q`, err.Error(), tt.wantErr)
				}
				return
			}

			if err := IsEqualTree(cp.Default, tt.args[len(tt.args)-2], tt.args[len(tt.args)-1]); err != nil {
				t.Errorf(`IsEqualTree(cp.Default, tt.args[1], tt.args[2]) = %q, not nil`, err)
			}
		})
	}
}

// TestCpSrcDirectory tests copying source to destination without recursive
// cmd-line equivalent: cp ~/dir ~/dir2
func TestCpSrcDirectory(t *testing.T) {
	tempDir := t.TempDir()
	tempDirTwo := t.TempDir()

	// capture log output to verify
	var logBytes bytes.Buffer
	var in bufio.Reader

	if err := run([]string{tempDir, tempDirTwo}, &logBytes, &in); err == nil {
		t.Fatal("cp succeeded when asked to copy a directory without -r")
	}

	outString := fmt.Sprintf("cp: -r not specified, omitting directory %s", tempDir)
	capturedString := logBytes.String()
	if !strings.Contains(capturedString, outString) {
		t.Fatal("strings.Contains(capturedString, outString) = false, not true")
	}
}

// TestCpRecursive tests the recursive mode copy
// Copy dir hierarchies src-dir to dst-dir
// whose src-dir and dst-dir already exists
// cmd-line equivalent: $ cp -R src-dir/ dst-dir/
func TestCpRecursive(t *testing.T) {
	tempDir := t.TempDir()

	srcDir := filepath.Join(tempDir, "src")
	if err := os.Mkdir(srcDir, 0o755); err != nil {
		t.Fatalf(`os.Mkdir(srcDir, 0o755) = %q, not nil`, err)
	}
	dstDir := filepath.Join(tempDir, "dst-exists")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatalf(`os.Mkdir(dstDir, 0o755) = %q, not nil`, err)
	}

	if err := createFilesTree(srcDir, maxDirDepth, 0); err != nil {
		t.Fatalf(`createFilesTree(srcDir, maxDirDepth, 0) = %q, not nil`, err)
	}

	t.Run("existing-dst-dir", func(t *testing.T) {
		var out bytes.Buffer
		var in bufio.Reader
		if err := run([]string{"-r", srcDir, dstDir}, &out, &in); err != nil {
			t.Fatalf(`run([]string{"-r",srcDir, dstDir}, &out, &in) = %q, not nil`, err)
		}
		// Because dstDir already existed, a new dir was created inside it.
		realDestination := filepath.Join(dstDir, filepath.Base(srcDir))
		if err := IsEqualTree(cp.Default, srcDir, realDestination); err != nil {
			t.Fatalf(`IsEqualTree(cp.Default, srcDir, realDestination) = %q, not nil`, err)
		}
	})

	t.Run("non-existing-dst-dir", func(t *testing.T) {
		var out bytes.Buffer
		var in bufio.Reader
		notExistDstDir := filepath.Join(tempDir, "dst-does-not-exist")
		if err := run([]string{"-r", srcDir, notExistDstDir}, &out, &in); err != nil {
			t.Fatalf(`run([]string{"-r",srcDir, notExistDstDir}, &out, &in) = %q, not nil`, err)
		}

		if err := IsEqualTree(cp.Default, srcDir, notExistDstDir); err != nil {
			t.Fatalf(`IsEqualTree(cp.Default, srcDir, notExistDstDir) = %q, not nil`, err)
		}
	})
}

// Other test to verify the CopyRecursive
// whose dir$n and dst-dir already exists
// cmd-line equivalent: $ cp -R dir1/ dir2/ dir3/ dst-dir/
//
// dst-dir will content dir{1, 3}
// $ dst-dir/
// ..	dir1/
// ..	dir2/
// ..   dir3/
func TestCpRecursiveMultiple(t *testing.T) {
	tempDir := t.TempDir()

	dstTest := filepath.Join(tempDir, "destination")
	if err := os.Mkdir(dstTest, 0o755); err != nil {
		t.Fatalf(`os.Mkdir(dstTest, 0o755) = %q, not nil`, err)
	}

	// create multiple random directories sources
	srcDirs := []string{}
	for range maxDirDepth {
		srcTest := t.TempDir()

		if err := createFilesTree(srcTest, maxDirDepth, 0); err != nil {
			t.Fatalf(`createFilesTree(srcTest, maxDirDepth, 0) = %q, not nil`, err)
		}

		srcDirs = append(srcDirs, srcTest)

	}
	var out bytes.Buffer
	var in bufio.Reader
	args := []string{"-r"}
	args = append(args, srcDirs...)
	args = append(args, dstTest)
	if err := run(args, &out, &in); err != nil {
		t.Fatalf(`run(args, &out, &in) = %q, not nil`, err)
	}
	// Make sure we can do it twice.
	args = []string{"-rf"}
	args = append(args, srcDirs...)
	args = append(args, dstTest)
	if err := run(args, &out, &in); err != nil {
		t.Fatalf(`run(args, &out, &in) = %q, not nil`, err)
	}
	for _, src := range srcDirs {
		_, srcFile := filepath.Split(src)

		dst := filepath.Join(dstTest, srcFile)
		if err := IsEqualTree(cp.Default, src, dst); err != nil {
			t.Fatalf(`IsEqualTree(cp.Default, src, dst) = %q, not nil`, err)
		}
	}
}

// using -P don't follow symlinks, create other symlink
// cmd-line equivalent: $ cp -P symlink symlink-copy
func TestCpSymlink(t *testing.T) {
	tempDir := t.TempDir()

	f, err := randomFile(tempDir, "src-")
	if err != nil {
		t.Fatalf(`randomFile(tempDir, "src-") = %q, not nil`, err)
	}
	defer f.Close()

	srcFpath := f.Name()
	srcFname := filepath.Base(srcFpath)

	newName := filepath.Join(tempDir, srcFname+"_link")
	if err := os.Symlink(srcFname, newName); err != nil {
		t.Skipf("skipping symlink test: %v", err)
	}

	t.Run("no-follow-symlink", func(t *testing.T) {
		var out bytes.Buffer
		var in bufio.Reader

		dst := filepath.Join(tempDir, "dst-no-follow")
		if err := run([]string{"-P", newName, dst}, &out, &in); err != nil {
			t.Fatalf(`run([]string{"-P", newName, dst}, &out, &in) = %q, not nil`, err)
		}
		if err := IsEqualTree(cp.NoFollowSymlinks, newName, dst); err != nil {
			t.Fatalf(`IsEqualTree(cp.NoFollowSymlinks, newName, dst) =%q, not nil`, err)
		}
	})

	t.Run("follow-symlink", func(t *testing.T) {
		var out bytes.Buffer
		var in bufio.Reader

		dst := filepath.Join(tempDir, "dst-follow")
		if err := run([]string{newName, dst}, &out, &in); err != nil {
			t.Fatalf(`run([]string{newName, dst}, &out, &in) =%q, not nil`, err)
		}
		if err := IsEqualTree(cp.Default, newName, dst); err != nil {
			t.Fatalf(`IsEqualTree(cp.Default, newName, dst) = %q, not nil`, err)
		}
	})
}

// isEqualFile compare two files by checksum
func isEqualFile(fpath1, fpath2 string) error {
	file1, err := os.Open(fpath1)
	if err != nil {
		return err
	}
	defer file1.Close()
	file2, err := os.Open(fpath2)
	if err != nil {
		return err
	}
	defer file2.Close()

	if !uio.ReaderAtEqual(file1, file2) {
		return fmt.Errorf("%q and %q do not have equal content", fpath1, fpath2)
	}
	return nil
}

func readDirNames(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var basenames []string
	for _, entry := range entries {
		basenames = append(basenames, entry.Name())
	}
	return basenames, nil
}

func stat(o cp.Options, path string) (os.FileInfo, error) {
	if o.NoFollowSymlinks {
		return os.Lstat(path)
	}
	return os.Stat(path)
}

// IsEqualTree compare the content in the file trees in src and dst paths
func IsEqualTree(o cp.Options, src, dst string) error {
	srcInfo, err := stat(o, src)
	if err != nil {
		return err
	}
	dstInfo, err := stat(o, dst)
	if err != nil {
		return err
	}
	if sm, dm := srcInfo.Mode()&os.ModeType, dstInfo.Mode()&os.ModeType; sm != dm {
		return fmt.Errorf("mismatched mode: %q has mode %s while %q has mode %s", src, sm, dst, dm)
	}

	switch {
	case srcInfo.Mode().IsDir():
		srcEntries, err := readDirNames(src)
		if err != nil {
			return err
		}
		dstEntries, err := readDirNames(dst)
		if err != nil {
			return err
		}
		// os.ReadDir guarantees these are sorted.
		if !slices.Equal(srcEntries, dstEntries) {
			return fmt.Errorf("directory contents did not match:\n%q had %v\n%q had %v", src, srcEntries, dst, dstEntries)
		}
		for _, basename := range srcEntries {
			if err := IsEqualTree(o, filepath.Join(src, basename), filepath.Join(dst, basename)); err != nil {
				return err
			}
		}
		return nil

	case srcInfo.Mode().IsRegular():
		return isEqualFile(src, dst)

	case srcInfo.Mode()&os.ModeSymlink == os.ModeSymlink:
		srcTarget, err := os.Readlink(src)
		if err != nil {
			return err
		}
		dstTarget, err := os.Readlink(dst)
		if err != nil {
			return err
		}
		if srcTarget != dstTarget {
			return fmt.Errorf("target mismatch: symlink %q had target %q, while %q had target %q", src, srcTarget, dst, dstTarget)
		}
		return nil

	default:
		return fmt.Errorf("unsupported mode: %s", srcInfo.Mode())
	}
}

func TestMainInvocation(t *testing.T) {
	ctx := context.Background()

	t.Run("basic copy with path resolution", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "src.txt")
		if err := os.WriteFile(src, []byte("hello copy"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		ioEnv := core.IO{Dir: dir, Stderr: &stderr}
		code := Main(ctx, ioEnv, "src.txt", "dst.txt")
		if code != 0 {
			t.Fatalf("Main returned %d, want 0; stderr=%s", code, stderr.String())
		}
		dstContent, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
		if err != nil {
			t.Fatalf("destination file not read: %v", err)
		}
		if string(dstContent) != "hello copy" {
			t.Errorf("content = %q, want 'hello copy'", string(dstContent))
		}
	})

	t.Run("copy to directory", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "subdir")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		src := filepath.Join(dir, "file1.txt")
		if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		ioEnv := core.IO{Dir: dir, Stderr: &stderr}
		code := Main(ctx, ioEnv, "file1.txt", "subdir")
		if code != 0 {
			t.Fatalf("Main returned %d, want 0; stderr=%s", code, stderr.String())
		}
		copiedContent, err := os.ReadFile(filepath.Join(sub, "file1.txt"))
		if err != nil {
			t.Fatalf("copied file not found: %v", err)
		}
		if string(copiedContent) != "data" {
			t.Errorf("copied content = %q, want 'data'", string(copiedContent))
		}
	})

	t.Run("recursive copy", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "tree")
		if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "sub", "leaf.txt"), []byte("leaf"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		ioEnv := core.IO{Dir: dir, Stderr: &stderr}
		code := Main(ctx, ioEnv, "-r", "tree", "tree_copy")
		if code != 0 {
			t.Fatalf("Main returned %d, want 0; stderr=%s", code, stderr.String())
		}
		copiedLeaf, err := os.ReadFile(filepath.Join(dir, "tree_copy", "sub", "leaf.txt"))
		if err != nil {
			t.Fatalf("copied leaf not found: %v", err)
		}
		if string(copiedLeaf) != "leaf" {
			t.Errorf("copied leaf = %q, want 'leaf'", string(copiedLeaf))
		}
	})

	t.Run("non-existent source returns 1", func(t *testing.T) {
		dir := t.TempDir()
		var stderr bytes.Buffer
		ioEnv := core.IO{Dir: dir, Stderr: &stderr}
		code := Main(ctx, ioEnv, "nosuchfile", "dest")
		if code != 1 {
			t.Fatalf("expected code 1 on missing source, got %d", code)
		}
	})

	t.Run("help flag returns 0", func(t *testing.T) {
		var stderr bytes.Buffer
		ioEnv := core.IO{Stderr: &stderr}
		code := Main(ctx, ioEnv, "--help")
		if code != 0 {
			t.Fatalf("expected code 0 on --help, got %d", code)
		}
	})
}
