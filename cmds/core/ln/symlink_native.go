//go:build !windows && !wasip1

package ln

import "os"

func createSymlink(target, link string, targetKind uint32) error {
	return os.Symlink(target, link)
}
