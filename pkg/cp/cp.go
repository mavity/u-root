// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cp implements file and directory copying.
package cp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrSkip = errors.New("skip")
var errPlatformUnsupported = errors.New("operation is not supported on this platform")

type SymlinkMode uint8

const (
	SymlinkDefault SymlinkMode = iota
	SymlinkNever
	SymlinkAlways
	SymlinkCommandLine
)

type BackupMode uint8

const (
	BackupNone BackupMode = iota
	BackupSimple
	BackupNumbered
	BackupExisting
)

type ReflinkMode uint8

const (
	ReflinkAuto ReflinkMode = iota
	ReflinkAlways
	ReflinkNever
)

type SparseMode uint8

const (
	SparseAuto SparseMode = iota
	SparseAlways
	SparseNever
)

type UpdateMode uint8

const (
	UpdateAll UpdateMode = iota
	UpdateOlder
	UpdateNone
	UpdateNoneFail
)

type Attributes struct {
	Mode       bool
	Ownership  bool
	Timestamps bool
	Links      bool
	Xattrs     bool
	Context    bool
}

var DefaultAttributes = Attributes{Mode: true, Ownership: true, Timestamps: true}
var AllAttributes = Attributes{Mode: true, Ownership: true, Timestamps: true, Links: true, Xattrs: true, Context: true}

type Options struct {
	Context              context.Context
	State                *State
	Recursive            bool
	Symlinks             SymlinkMode
	NoFollowSymlinks     bool
	Preserve             Attributes
	BestEffortAttributes bool
	AttributesOnly       bool
	CopyContents         bool
	OneFileSystem        bool
	KeepDirectorySymlink bool
	ContextLabel         string
	Force                bool
	RemoveDestination    bool
	HardLink             bool
	SymbolicLink         bool
	Reflink              ReflinkMode
	Sparse               SparseMode
	Backup               BackupMode
	BackupSuffix         string
	Update               UpdateMode
	Debug                io.Writer
	PreCallback          func(src, dst string, srcfi os.FileInfo) error
	PostCallback         func(src, dst string)
}

var Default = Options{}

var NoFollowSymlinks = Options{NoFollowSymlinks: true, Symlinks: SymlinkNever}

type copiedLink struct {
	info os.FileInfo
	dst  string
}

type State struct {
	links []copiedLink
}

type copyState struct {
	*State
	rootDevice uint64
	rootHasDev bool
	ancestors  []os.FileInfo
}

func (o Options) context() context.Context {
	if o.Context == nil {
		return context.Background()
	}
	return o.Context
}

func (o Options) shouldFollow(path string, root, recursive bool) bool {
	if o.NoFollowSymlinks || o.Symlinks == SymlinkNever {
		return false
	}
	switch o.Symlinks {
	case SymlinkAlways:
		return true
	case SymlinkCommandLine:
		return root
	default:
		if o.HardLink {
			return true
		}
		return !recursive
	}
}

func (o Options) Copy(src, dst string) error {
	state := o.newState()
	return o.copyPath(src, dst, false, true, state)
}

func (o Options) CopyTree(src, dst string) error {
	if err := o.validateDestination(src, dst); err != nil {
		return err
	}
	state := o.newState()
	return o.copyPath(src, dst, true, true, state)
}

func (o Options) newState() *copyState {
	state := o.State
	if state == nil {
		state = &State{}
	}
	return &copyState{State: state}
}

func Copy(src, dst string) error {
	return Default.Copy(src, dst)
}

func CopyTree(src, dst string) error {
	return Default.CopyTree(src, dst)
}

func (o Options) copyPath(src, dst string, recursive, root bool, state *copyState) error {
	if err := o.context().Err(); err != nil {
		return err
	}

	linkInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	info := linkInfo
	follow := linkInfo.Mode()&os.ModeSymlink != 0 && o.shouldFollow(src, root, recursive)
	if follow {
		info, err = os.Stat(src)
		if err != nil {
			return err
		}
	}

	if o.PreCallback != nil {
		if err := o.PreCallback(src, dst, info); errors.Is(err, ErrSkip) {
			return nil
		} else if err != nil {
			return err
		}
	}

	switch {
	case info.IsDir():
		if !recursive {
			return fmt.Errorf("omitting directory %s", src)
		}
		return o.copyDirectory(src, dst, info, root, state)
	case linkInfo.Mode()&os.ModeSymlink != 0 && !follow:
		return o.copySymlink(src, dst, info)
	case info.Mode().IsRegular():
		return o.copyRegular(src, dst, info, state, follow)
	case !recursive || o.CopyContents:
		return o.copySpecialContents(src, dst, info)
	default:
		return o.copySpecialFile(src, dst, info)
	}
}

func (o Options) copyDirectory(src, dst string, info os.FileInfo, root bool, state *copyState) error {
	for _, ancestor := range state.ancestors {
		if os.SameFile(ancestor, info) {
			return fmt.Errorf("cannot copy directory %q through a symbolic-link loop", src)
		}
	}

	dev, hasDev := fileDevice(info)
	if root {
		state.rootDevice, state.rootHasDev = dev, hasDev
		if o.OneFileSystem && !hasDev {
			return fmt.Errorf("--one-file-system: %w", errPlatformUnsupported)
		}
	} else if o.OneFileSystem && state.rootHasDev && hasDev && dev != state.rootDevice {
		return nil
	}

	dstInfo, err := os.Lstat(dst)
	switch {
	case err == nil && dstInfo.Mode()&os.ModeSymlink != 0 && o.KeepDirectorySymlink:
		targetInfo, statErr := os.Stat(dst)
		if statErr == nil && targetInfo.IsDir() {
			dstInfo = targetInfo
		} else {
			return fmt.Errorf("destination %q is not a directory", dst)
		}
	case err == nil && !dstInfo.IsDir():
		return fmt.Errorf("cannot overwrite non-directory %q with directory %q", dst, src)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}

	state.ancestors = append(state.ancestors, info)
	defer func() { state.ancestors = state.ancestors[:len(state.ancestors)-1] }()

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := o.context().Err(); err != nil {
			return err
		}
		childSrc := filepath.Join(src, entry.Name())
		childDst := filepath.Join(dst, entry.Name())
		if err := o.copyPath(childSrc, childDst, true, false, state); err != nil {
			return err
		}
	}

	if err := o.preserveMetadata(src, dst, info, false); err != nil {
		return err
	}
	if o.PostCallback != nil {
		o.PostCallback(src, dst)
	}
	return nil
}

func (o Options) copySymlink(src, dst string, info os.FileInfo) error {
	if err := o.prepareDestination(src, dst, info, true); err != nil {
		return err
	}
	if err := o.removeExistingForLink(dst); err != nil {
		return err
	}
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if err := os.Symlink(target, dst); err != nil {
		return err
	}
	if err := o.preserveMetadata(src, dst, info, true); err != nil {
		return err
	}
	if o.PostCallback != nil {
		o.PostCallback(src, dst)
	}
	return nil
}

func (o Options) copyRegular(src, dst string, info os.FileInfo, state *copyState, followed bool) error {
	reportSrc := src
	if o.Force && o.Backup != BackupNone {
		dstInfo, statErr := os.Lstat(dst)
		same := statErr == nil && os.SameFile(info, dstInfo)
		if statErr == nil && dstInfo.Mode()&os.ModeSymlink != 0 {
			if followed, followErr := os.Stat(dst); followErr == nil {
				same = same || os.SameFile(info, followed)
			}
		}
		if same {
			backup, err := o.backupPath(dst)
			if err != nil {
				return err
			}
			if o.Backup == BackupSimple {
				if err := os.Remove(backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			if err := os.Rename(dst, backup); err != nil {
				return err
			}
			if filepath.Clean(src) == filepath.Clean(dst) {
				src = backup
				info, err = os.Stat(backup)
				if err != nil {
					return err
				}
			}
		} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}
	}

	if o.Preserve.Links {
		for _, link := range state.links {
			if os.SameFile(link.info, info) {
				if err := o.prepareDestination(src, dst, info, false); err != nil {
					return err
				}
				if err := os.Link(link.dst, dst); err != nil {
					return err
				}
				if o.PostCallback != nil {
					o.PostCallback(reportSrc, dst)
				}
				return nil
			}
		}
	}

	if err := o.prepareDestination(src, dst, info, false); err != nil {
		return err
	}
	if o.SymbolicLink {
		if err := o.removeExistingForLink(dst); err != nil {
			return err
		}
		target, err := sourceLinkTarget(src, dst, followed)
		if err != nil {
			return err
		}
		if err := os.Symlink(target, dst); err != nil {
			return err
		}
	} else if o.HardLink {
		if err := o.removeExistingForLink(dst); err != nil {
			return err
		}
		linkSource, err := effectiveSource(src, followed)
		if err != nil {
			return err
		}
		if err := os.Link(linkSource, dst); err != nil {
			if o.Force {
				_ = os.Remove(dst)
				if err = os.Link(linkSource, dst); err != nil {
					return err
				}
			} else {
				return err
			}
		}
	} else if err := o.copyRegularContents(src, dst, info); err != nil {
		return err
	}

	if !o.HardLink && !o.SymbolicLink {
		if err := o.preserveMetadata(src, dst, info, false); err != nil {
			return err
		}
	}
	if o.Preserve.Links {
		state.links = append(state.links, copiedLink{info: info, dst: dst})
	}
	if o.PostCallback != nil {
		o.PostCallback(reportSrc, dst)
	}
	return nil
}

func (o Options) prepareDestination(src, dst string, srcInfo os.FileInfo, symlink bool) error {
	dstInfo, err := os.Lstat(dst)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if dstInfo.IsDir() {
		return fmt.Errorf("cannot overwrite directory %q with non-directory %q", dst, src)
	}
	if dstInfo.Mode()&os.ModeSymlink != 0 && o.Backup == BackupNone && !o.RemoveDestination && !o.HardLink && !o.SymbolicLink && os.Getenv("POSIXLY_CORRECT") == "" {
		if _, statErr := os.Stat(dst); errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("refusing to overwrite dangling symbolic link %q", dst)
		}
	}
	if o.AttributesOnly {
		return nil
	}
	sameFile := os.SameFile(srcInfo, dstInfo)
	if srcInfo.Mode()&os.ModeSymlink == 0 && dstInfo.Mode()&os.ModeSymlink != 0 {
		if followed, statErr := os.Stat(dst); statErr == nil {
			sameFile = sameFile || os.SameFile(srcInfo, followed)
		}
	}
	if sameFile {
		return fmt.Errorf("%q and %q are the same file", src, dst)
	}

	if o.Backup != BackupNone {
		backup, err := o.backupPath(dst)
		if err != nil {
			return err
		}
		if o.Backup == BackupSimple {
			if err := os.Remove(backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		if err := os.Rename(dst, backup); err != nil {
			return err
		}
		return nil
	}
	if o.RemoveDestination {
		return os.Remove(dst)
	}
	return nil
}

func (o Options) copyRegularContents(src, dst string, info os.FileInfo) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	mode := info.Mode().Perm()
	openFlags := os.O_RDWR | os.O_CREATE
	if o.AttributesOnly {
		openFlags = os.O_RDONLY | os.O_CREATE
	} else {
		openFlags |= os.O_TRUNC
	}
	dstFile, err := os.OpenFile(dst, openFlags, mode)
	if err != nil && o.Force {
		if removeErr := os.Remove(dst); removeErr == nil {
			dstFile, err = os.OpenFile(dst, openFlags, mode)
		}
	}
	if err != nil {
		return err
	}
	defer dstFile.Close()

	copyMethod := "copied"
	if !o.AttributesOnly {
		if o.Reflink != ReflinkNever {
			cloneErr := cloneFile(dstFile, srcFile)
			switch {
			case cloneErr == nil:
				copyMethod = "reflinked"
			case o.Reflink == ReflinkAlways:
				return cloneErr
			default:
				if _, err := dstFile.Seek(0, io.SeekStart); err != nil {
					return err
				}
				if _, err := srcFile.Seek(0, io.SeekStart); err != nil {
					return err
				}
				if err := o.copyData(dstFile, srcFile, info.Size(), o.shouldCreateSparse(info)); err != nil {
					return err
				}
			}
		} else if err := o.copyData(dstFile, srcFile, info.Size(), o.shouldCreateSparse(info)); err != nil {
			return err
		}
	}
	if o.Debug != nil {
		fmt.Fprintf(o.Debug, "cp: %q: %s (sparse=%t)\n", src, copyMethod, o.shouldCreateSparse(info))
	}
	return nil
}

func (o Options) shouldCreateSparse(info os.FileInfo) bool {
	switch o.Sparse {
	case SparseAlways:
		return true
	case SparseAuto:
		return fileIsSparse(info)
	default:
		return false
	}
}

func (o Options) copyData(dst io.WriteSeeker, src io.Reader, size int64, sparse bool) error {
	if !sparse {
		_, err := io.Copy(dst, contextReader{o.context(), src})
		return err
	}

	const blockSize = 64 * 1024
	buf := make([]byte, blockSize)
	var written int64
	for {
		if err := o.context().Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if allZero(chunk) {
				if _, err := dst.Seek(int64(n), io.SeekCurrent); err != nil {
					return err
				}
			} else {
				if _, err := dst.Write(chunk); err != nil {
					return err
				}
			}
			written += int64(n)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return readErr
		}
	}
	if size < written {
		size = written
	}
	currentSize, err := dst.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if currentSize < size {
		if _, err := dst.Seek(size-1, io.SeekStart); err != nil {
			return err
		}
		if n, err := dst.Write([]byte{0}); err != nil {
			return err
		} else if n != 1 {
			return io.ErrShortWrite
		}
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func effectiveSource(src string, followed bool) (string, error) {
	if !followed {
		return src, nil
	}
	info, err := os.Lstat(src)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return src, nil
	}
	return filepath.EvalSymlinks(src)
}

func sourceLinkTarget(src, dst string, followed bool) (string, error) {
	source, err := effectiveSource(src, followed)
	if err != nil {
		return "", err
	}
	absSource, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	return filepath.Rel(filepath.Dir(dst), absSource)
}

func (o Options) copySpecialContents(src, dst string, info os.FileInfo) error {
	if err := o.prepareDestination(src, dst, info, false); err != nil {
		return err
	}
	if o.AttributesOnly {
		out, err := os.OpenFile(dst, os.O_RDONLY|os.O_CREATE, info.Mode().Perm())
		if err != nil {
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		if err := o.preserveMetadata(src, dst, info, false); err != nil {
			return err
		}
		if o.PostCallback != nil {
			o.PostCallback(src, dst)
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(out, contextReader{o.context(), in}); err != nil {
		return err
	}
	if err := o.preserveMetadata(src, dst, info, false); err != nil {
		return err
	}
	if o.PostCallback != nil {
		o.PostCallback(src, dst)
	}
	return nil
}

func (o Options) copySpecialFile(src, dst string, info os.FileInfo) error {
	if err := o.prepareDestination(src, dst, info, false); err != nil {
		return err
	}
	if err := o.removeExistingForLink(dst); err != nil {
		return err
	}
	if err := createSpecial(dst, info); err != nil {
		return err
	}
	if err := o.preserveMetadata(src, dst, info, false); err != nil {
		return err
	}
	if o.PostCallback != nil {
		o.PostCallback(src, dst)
	}
	return nil
}

func (o Options) removeExistingForLink(dst string) error {
	if o.Backup != BackupNone || o.RemoveDestination {
		return nil
	}
	if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (o Options) preserveMetadata(src, dst string, info os.FileInfo, symlink bool) error {
	if o.Preserve.Ownership {
		if err := preserveOwner(info, dst, symlink); err != nil {
			if !o.BestEffortAttributes || !errors.Is(err, errPlatformUnsupported) {
				return err
			}
		}
	}
	if o.Preserve.Mode && !symlink {
		if err := os.Chmod(dst, info.Mode()); err != nil {
			return err
		}
	}
	if o.Preserve.Timestamps {
		atime, mtime := fileTimes(info)
		if err := setFileTimes(dst, atime, mtime, symlink); err != nil {
			if !o.BestEffortAttributes || !errors.Is(err, errPlatformUnsupported) {
				return err
			}
		}
	}
	if o.Preserve.Xattrs {
		if err := copyXattrs(src, dst, symlink); err != nil && !o.BestEffortAttributes {
			return err
		}
	}
	if o.Preserve.Context && o.ContextLabel == "" {
		if err := copySecurityContext(src, dst, symlink); err != nil && !o.BestEffortAttributes {
			return err
		}
	}
	if o.ContextLabel != "" {
		if err := setFileContext(dst, o.ContextLabel); err != nil {
			return err
		}
	}
	return nil
}

func (o Options) backupPath(dst string) (string, error) {
	suffix := o.BackupSuffix
	if suffix == "" {
		suffix = os.Getenv("SIMPLE_BACKUP_SUFFIX")
	}
	if suffix == "" {
		suffix = "~"
	}
	makeNumbered := o.Backup == BackupNumbered
	if o.Backup == BackupExisting {
		makeNumbered = hasNumberedBackup(dst, suffix)
	}
	if !makeNumbered {
		return dst + suffix, nil
	}
	for n := 1; ; n++ {
		path := fmt.Sprintf("%s.~%d%s", dst, n, suffix)
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			return path, nil
		} else if err != nil {
			return "", err
		}
	}
}

func hasNumberedBackup(dst, suffix string) bool {
	matches, _ := filepath.Glob(dst + ".~*" + suffix)
	for _, path := range matches {
		base := strings.TrimSuffix(strings.TrimPrefix(path, dst+".~"), suffix)
		if _, err := strconv.Atoi(base); err == nil {
			return true
		}
	}
	return false
}

func (o Options) validateDestination(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if srcInfo.IsDir() {
		srcAbs, srcErr := filepath.Abs(src)
		dstAbs, dstErr := filepath.Abs(dst)
		if srcErr == nil && dstErr == nil {
			rel, relErr := filepath.Rel(srcAbs, dstAbs)
			if relErr == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
				return fmt.Errorf("cannot copy %q into itself, %q", src, dst)
			}
		}
	}
	dstInfo, err := os.Stat(dst)
	if err == nil && srcInfo.IsDir() && dstInfo.IsDir() {
		rel, relErr := filepath.Rel(src, dst)
		if relErr == nil && rel == "." {
			return fmt.Errorf("cannot copy %q into itself, %q", src, dst)
		}
	}
	return nil
}

func (o Options) CopyTreeChecked(src, dst string) error {
	if err := o.validateDestination(src, dst); err != nil {
		return err
	}
	return o.CopyTree(src, dst)
}
