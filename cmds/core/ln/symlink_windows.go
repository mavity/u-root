//go:build windows && !wasip1

package ln

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var createSymbolicLinkW = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateSymbolicLinkW")

func createSymlink(target, link string, targetKind uint32) error {
	if targetKind == 0 {
		statPath := target
		if !filepath.IsAbs(statPath) {
			statPath = filepath.Join(filepath.Dir(link), statPath)
		}
		info, err := os.Stat(statPath)
		switch {
		case err == nil && info.IsDir():
			targetKind = 2
		case err == nil, os.IsNotExist(err):
			targetKind = 1
		default:
			return err
		}
	}
	if targetKind != 2 {
		return os.Symlink(target, link)
	}
	linkName, err := syscall.UTF16PtrFromString(link)
	if err != nil {
		return err
	}
	targetName, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	flags := uintptr(0x2)
	if targetKind == 2 {
		flags |= 0x1
	}
	result, _, callErr := createSymbolicLinkW.Call(
		uintptr(unsafe.Pointer(linkName)),
		uintptr(unsafe.Pointer(targetName)),
		flags,
	)
	if result == 0 && callErr == syscall.Errno(87) {
		result, _, callErr = createSymbolicLinkW.Call(
			uintptr(unsafe.Pointer(linkName)),
			uintptr(unsafe.Pointer(targetName)),
			flags&^uintptr(0x2),
		)
	}
	if result == 0 {
		return &os.LinkError{Op: "symlink", Old: target, New: link, Err: callErr}
	}
	return nil
}
