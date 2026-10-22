package cp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCopyTreePreservesLinksAndSymlinks(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("shared payload")
	first := filepath.Join(src, "first")
	if err := os.WriteFile(first, data, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(first, filepath.Join(src, "second")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := os.Symlink("first", filepath.Join(src, "alias")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	opts := Options{
		Symlinks: SymlinkNever,
		Preserve: Attributes{Links: true},
	}
	if err := opts.CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	firstInfo, err := os.Stat(filepath.Join(dst, "first"))
	if err != nil {
		t.Fatal(err)
	}
	secondInfo, err := os.Stat(filepath.Join(dst, "second"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(firstInfo, secondInfo) {
		t.Fatal("hard-link relationship was not preserved")
	}
	linkInfo, err := os.Lstat(filepath.Join(dst, "alias"))
	if err != nil {
		t.Fatal(err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symbolic link was copied as its target")
	}
	target, err := os.Readlink(filepath.Join(dst, "alias"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "first" {
		t.Fatalf("symbolic-link target = %q, want %q", target, "first")
	}
}

func TestCopyTreeDereferencePolicies(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "target"), []byte("followed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(src, "link")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	if err := (Options{Symlinks: SymlinkAlways}).CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(dst, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("dereferenced entry has mode %s, want regular file", info.Mode())
	}
	contents, err := os.ReadFile(filepath.Join(dst, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "followed" {
		t.Fatalf("contents = %q, want followed", contents)
	}
}

func TestCopyTreeCommandLineDereference(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "file"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "source-link")
	if err := os.Symlink(target, src); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	dst := filepath.Join(root, "copy")
	if err := (Options{Symlinks: SymlinkCommandLine}).CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("command-line symlink was not followed, mode=%s", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(dst, "file")); err != nil {
		t.Fatalf("followed directory contents missing: %v", err)
	}
}

func TestCopyTreePreservesModeAndModificationTime(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}
	wantTime := time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(src, wantTime, wantTime); err != nil {
		t.Fatal(err)
	}

	opts := Options{Preserve: Attributes{Mode: true, Timestamps: true}}
	if err := opts.Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && srcInfo.Mode().Perm() != dstInfo.Mode().Perm() {
		t.Fatalf("mode = %v, want %v", dstInfo.Mode().Perm(), srcInfo.Mode().Perm())
	}
	if !dstInfo.ModTime().Equal(srcInfo.ModTime()) {
		t.Fatalf("modification time = %v, want %v", dstInfo.ModTime(), srcInfo.ModTime())
	}
}

func TestCopyAttributesOnlyLeavesExistingContents(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("source data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("destination data"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{AttributesOnly: true, Preserve: Attributes{Mode: true}}
	if err := opts.Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "destination data" {
		t.Fatalf("destination contents = %q, want unchanged", got)
	}
}

func TestCopyBackupAndSuffix(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	backup := dst + ".bak"
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{Backup: BackupSimple, BackupSuffix: ".bak"}
	if err := opts.Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("destination = %q, want new", got)
	}
	got, err = os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("backup = %q, want old", got)
	}
}

func TestCopySparseAndReflinkFallback(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	data := append(make([]byte, 128*1024), []byte("tail")...)
	data = append(data, make([]byte, 128*1024)...)
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Sparse: SparseAlways, Reflink: ReflinkAuto}
	if err := opts.Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("sparse/reflink copy changed file contents")
	}
}

func TestCopyHonorsCanceledContext(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (Options{Context: ctx}).Copy(src, dst)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Copy error = %v, want context.Canceled", err)
	}
}

type cancelAfterRead struct {
	cancel context.CancelFunc
	done   bool
}

func (r *cancelAfterRead) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	r.cancel()
	return copy(p, []byte("first chunk")), nil
}

func TestCopyDataChecksCancellationDuringCopy(t *testing.T) {
	root := t.TempDir()
	dst, err := os.Create(filepath.Join(root, "destination"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	ctx, cancel := context.WithCancel(context.Background())
	err = (Options{Context: ctx}).copyData(dst, &cancelAfterRead{cancel: cancel}, -1, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("copyData error = %v, want context.Canceled", err)
	}
}

func TestCopyTreeDefaultDoesNotFollowNestedSymlinks(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "target"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(src, "alias")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := (Options{}).CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(dst, "alias"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("recursive default followed a nested symbolic link")
	}
}

func TestCopyTreeRejectsDestinationInsideSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (Options{}).CopyTree(src, filepath.Join(src, "nested")); err == nil {
		t.Fatal("CopyTree succeeded with destination inside source")
	}
}

func TestCopyDataDoesNotChangeContentWhenSparse(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sparse")
	dst, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append(make([]byte, 128*1024), []byte("end")...)
	if err := (Options{}).copyData(dst, bytes.NewReader(want), int64(len(want)), true); err != nil {
		dst.Close()
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("sparse writer changed file contents")
	}
}
