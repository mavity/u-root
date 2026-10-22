//go:build linux

package cp

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCopySpecialFileAndExtendedAttributes(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	dstDir := filepath.Join(root, "dst")
	if err := os.Mkdir(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(srcDir, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "file")
	if err := os.WriteFile(src, []byte("xattr"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(src, "user.u-root-test", []byte("value"), 0); err != nil {
		t.Skipf("extended attributes unavailable: %v", err)
	}

	if err := (Options{}).CopyTree(srcDir, dstDir); err != nil {
		t.Fatal(err)
	}
	pipeInfo, err := os.Lstat(filepath.Join(dstDir, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if pipeInfo.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("copied special file mode=%s, want FIFO", pipeInfo.Mode())
	}

	attrDst := filepath.Join(root, "attr-copy")
	if err := (Options{Preserve: Attributes{Xattrs: true}}).Copy(src, attrDst); err != nil {
		t.Fatal(err)
	}
	value := make([]byte, len("value"))
	n, err := unix.Getxattr(attrDst, "user.u-root-test", value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(value[:n], []byte("value")) {
		t.Fatalf("copied xattr = %q, want value", value[:n])
	}
}

func TestSparseAlwaysCreatesHoles(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	data := append(make([]byte, 1024*1024), []byte("tail")...)
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (Options{Sparse: SparseAlways, Reflink: ReflinkNever}).Copy(src, dst); err != nil {
		t.Fatal(err)
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := dstInfo.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("filesystem statistics do not expose allocated blocks")
	}
	if stat.Blocks*512 >= dstInfo.Size()/2 {
		t.Skipf("filesystem does not report sparse allocation (blocks=%d size=%d)", stat.Blocks, dstInfo.Size())
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("sparse copy changed contents")
	}
}
