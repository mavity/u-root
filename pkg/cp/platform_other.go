//go:build !linux

package cp

import (
	"fmt"
	"os"
	"time"
)

func fileDevice(os.FileInfo) (uint64, bool) {
	return 0, false
}

func fileIsSparse(os.FileInfo) bool {
	return false
}

func fileTimes(info os.FileInfo) (time.Time, time.Time) {
	return info.ModTime(), info.ModTime()
}

func setFileTimes(path string, atime, mtime time.Time, symlink bool) error {
	if symlink {
		return setFileTimesNoFollow(path, atime, mtime)
	}
	return os.Chtimes(path, atime, mtime)
}

func preserveOwner(os.FileInfo, string, bool) error {
	return fmt.Errorf("preserve owner: %w", errPlatformUnsupported)
}

func copyXattrs(string, string, bool) error {
	return fmt.Errorf("copy extended attributes: %w", errPlatformUnsupported)
}

func copySecurityContext(string, string, bool) error {
	return fmt.Errorf("copy security context: %w", errPlatformUnsupported)
}

func setFileContext(string, string) error {
	return fmt.Errorf("set security context: %w", errPlatformUnsupported)
}

func createSpecial(string, os.FileInfo) error {
	return fmt.Errorf("create special file: %w", errPlatformUnsupported)
}

func cloneFile(*os.File, *os.File) error {
	return fmt.Errorf("reflink: %w", errPlatformUnsupported)
}
