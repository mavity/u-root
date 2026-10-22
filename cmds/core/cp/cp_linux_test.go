//go:build linux

package cpcmd

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCopyContentsCopiesFIFODataRecursively(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(src, "pipe")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}

	writerDone := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			fd, err := unix.Open(fifo, unix.O_WRONLY|unix.O_NONBLOCK, 0)
			if err == nil {
				file := os.NewFile(uintptr(fd), fifo)
				_, writeErr := file.Write([]byte("fifo payload"))
				closeErr := file.Close()
				writerDone <- errors.Join(writeErr, closeErr)
				return
			}
			if !errors.Is(err, unix.ENXIO) {
				writerDone <- err
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		writerDone <- errors.New("timed out waiting for FIFO reader")
	}()

	if err := run([]string{"-R", "--copy-contents", src, dst}, &bytes.Buffer{}, bufio.NewReader(bytes.NewReader(nil))); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("FIFO writer did not complete")
	}

	got, err := os.ReadFile(filepath.Join(dst, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fifo payload" {
		t.Fatalf("copied FIFO data = %q, want fifo payload", got)
	}
}
