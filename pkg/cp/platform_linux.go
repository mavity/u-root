//go:build linux

package cp

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func fileDevice(info os.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Dev), true
}

func fileIsSparse(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Size() > 0 && stat.Blocks*512 < info.Size()
}

func fileTimes(info os.FileInfo) (time.Time, time.Time) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.ModTime(), info.ModTime()
	}
	return time.Unix(stat.Atim.Sec, stat.Atim.Nsec), info.ModTime()
}

func setFileTimes(path string, atime, mtime time.Time, symlink bool) error {
	flags := 0
	if symlink {
		flags = unix.AT_SYMLINK_NOFOLLOW
	}
	times := []unix.Timespec{unix.NsecToTimespec(atime.UnixNano()), unix.NsecToTimespec(mtime.UnixNano())}
	return unix.UtimesNanoAt(unix.AT_FDCWD, path, times, flags)
}

func preserveOwner(info os.FileInfo, dst string, symlink bool) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("source ownership is unavailable")
	}
	if symlink {
		return os.Lchown(dst, int(stat.Uid), int(stat.Gid))
	}
	return os.Chown(dst, int(stat.Uid), int(stat.Gid))
}

func copyXattrs(src, dst string, symlink bool) error {
	list := unix.Listxattr
	get := unix.Getxattr
	set := unix.Setxattr
	if symlink {
		list = unix.Llistxattr
		get = unix.Lgetxattr
		set = unix.Lsetxattr
	}
	size, err := list(src, nil)
	if err != nil {
		return err
	}
	names := make([]byte, size)
	if _, err := list(src, names); err != nil {
		return err
	}
	var errs []error
	for _, name := range splitXattrNames(names) {
		valueSize, err := get(src, name, nil)
		if err != nil {
			errs = append(errs, fmt.Errorf("read xattr %q: %w", name, err))
			continue
		}
		value := make([]byte, valueSize)
		if _, err := get(src, name, value); err != nil {
			errs = append(errs, fmt.Errorf("read xattr %q: %w", name, err))
			continue
		}
		if err := set(dst, name, value, 0); err != nil {
			errs = append(errs, fmt.Errorf("set xattr %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

func copySecurityContext(src, dst string, symlink bool) error {
	get := unix.Getxattr
	set := unix.Setxattr
	if symlink {
		get = unix.Lgetxattr
		set = unix.Lsetxattr
	}
	size, err := get(src, "security.selinux", nil)
	if err != nil {
		return err
	}
	value := make([]byte, size)
	if _, err := get(src, "security.selinux", value); err != nil {
		return err
	}
	return set(dst, "security.selinux", value, 0)
}

func setFileContext(dst, label string) error {
	if label == "default" {
		return errors.New("setting the policy-default SELinux context is unavailable")
	}
	return unix.Setxattr(dst, "security.selinux", []byte(label), 0)
}

func createSpecial(dst string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot determine special-file device for %q", dst)
	}
	switch {
	case info.Mode()&os.ModeNamedPipe != 0:
		return unix.Mkfifo(dst, uint32(info.Mode().Perm()))
	case info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice != 0:
		return unix.Mknod(dst, uint32(unix.S_IFCHR)|uint32(info.Mode().Perm()), int(stat.Rdev))
	case info.Mode()&os.ModeDevice != 0:
		return unix.Mknod(dst, uint32(unix.S_IFBLK)|uint32(info.Mode().Perm()), int(stat.Rdev))
	default:
		return fmt.Errorf("unsupported file mode %s", info.Mode())
	}
}

func cloneFile(dst, src *os.File) error {
	return unix.IoctlFileClone(int(dst.Fd()), int(src.Fd()))
}

func splitXattrNames(data []byte) []string {
	var names []string
	for len(data) > 0 {
		end := 0
		for end < len(data) && data[end] != 0 {
			end++
		}
		if end > 0 {
			names = append(names, string(data[:end]))
		}
		if end == len(data) {
			break
		}
		data = data[end+1:]
	}
	return names
}
