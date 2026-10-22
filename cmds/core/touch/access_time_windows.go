//go:build windows

package touch

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func referenceAccessTime(info os.FileInfo) (time.Time, error) {
	stat, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, fmt.Errorf("unsupported stat metadata type %T", info.Sys())
	}
	return time.Unix(0, stat.LastAccessTime.Nanoseconds()), nil
}
