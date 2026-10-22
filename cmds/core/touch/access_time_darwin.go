//go:build darwin

package touch

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func referenceAccessTime(info os.FileInfo) (time.Time, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, fmt.Errorf("unsupported stat metadata type %T", info.Sys())
	}
	return time.Unix(stat.Atimespec.Sec, stat.Atimespec.Nsec), nil
}
