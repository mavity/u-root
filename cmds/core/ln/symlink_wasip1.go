//go:build wasip1

package ln

import "syscall"

func createSymlink(target, link string, targetKind uint32) error {
	return syscall.SymlinkWithType(target, link, targetKind)
}
