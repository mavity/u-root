//go:build wasip1

package cp

import (
	"syscall"
	"time"
)

const (
	atFDCWD           = -100
	atSymlinkNoFollow = 0x100
)

func setFileTimesNoFollow(path string, atime, mtime time.Time) error {
	times := []syscall.Timespec{
		{Sec: atime.Unix(), Nsec: int64(atime.Nanosecond())},
		{Sec: mtime.Unix(), Nsec: int64(mtime.Nanosecond())},
	}
	return syscall.UtimesNanoAt(atFDCWD, path, times, atSymlinkNoFollow)
}
