package cpcmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/u-root/u-root/pkg/cp"
)

func TestArchiveModeCopiesTreeAndLinks(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "tree")
	dst := filepath.Join(root, "copy")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file"), []byte("archive"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(src, "file"), filepath.Join(src, "hard-link")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := os.Symlink("file", filepath.Join(src, "link")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	if err := run([]string{"-a", src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "file"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "archive" {
		t.Fatalf("file contents = %q, want archive", got)
	}
	linkInfo, err := os.Lstat(filepath.Join(dst, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("-a dereferenced a symbolic link")
	}
	fileInfo, err := os.Stat(filepath.Join(dst, "file"))
	if err != nil {
		t.Fatal(err)
	}
	hardLinkInfo, err := os.Stat(filepath.Join(dst, "hard-link"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(fileInfo, hardLinkInfo) {
		t.Fatal("-a did not preserve the hard-link relationship")
	}
}

func TestNoClobberAndUpdate(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("destination"), 0o644); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(time.Hour)
	if err := os.Chtimes(dst, newer, newer); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"-n", src, dst},
		{"-u", src, dst},
		{"--update=none", src, dst},
	} {
		if err := run(args, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
			t.Fatalf("run(%v): %v", args, err)
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "destination" {
			t.Fatalf("run(%v) overwrote destination: %q", args, got)
		}
	}
	if err := run([]string{"--update=none-fail", src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err == nil {
		t.Fatal("--update=none-fail succeeded when skipping an existing destination")
	}
}

func TestBackupNumberedAndSuffix(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"--backup=numbered", "-S.bak", src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst + ".~1.bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("numbered backup = %q, want old", got)
	}
}

func TestTargetDirectoryNoTargetAndParents(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("targeted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--target-directory", target, src}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "source")); err != nil {
		t.Fatalf("target-directory copy missing: %v", err)
	}

	direct := filepath.Join(root, "direct")
	if err := run([]string{"-T", src, direct}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(direct); err != nil {
		t.Fatalf("no-target-directory copy missing: %v", err)
	}

	parentRoot := filepath.Join(root, "parent-target")
	if err := os.Mkdir(parentRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	relativeSource := filepath.Join("source-dir", "nested", "file")
	relativePath := filepath.Join(root, relativeSource)
	if err := os.MkdirAll(filepath.Dir(relativePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(relativePath, []byte("parented"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolve := func(path string) string {
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(root, path)
	}
	var out, errOut bytes.Buffer
	if err := runWithCtx(context.Background(), []string{"--path", "-t", parentRoot, relativeSource}, &out, &errOut, nil, resolve); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parentRoot, relativeSource)); err != nil {
		t.Fatalf("--parents copy missing: %v", err)
	}
}

func TestSymlinkOptionsUseLastOption(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "target"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(src, "link")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	for _, test := range []struct {
		name string
		args []string
		link bool
	}{
		{name: "later-P", args: []string{"-R", "-L", "-P"}, link: true},
		{name: "later-L", args: []string{"-R", "-P", "-L"}, link: false},
		{name: "archive-then-L", args: []string{"-a", "-L"}, link: false},
		{name: "L-then-archive", args: []string{"-L", "-a"}, link: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dst := filepath.Join(root, test.name)
			args := append(append([]string{}, test.args...), src, dst)
			if err := run(args, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(filepath.Join(dst, "link"))
			if err != nil {
				t.Fatal(err)
			}
			isLink := info.Mode()&os.ModeSymlink != 0
			if isLink != test.link {
				t.Fatalf("copied link mode=%s, symbolic=%t; want symbolic=%t", info.Mode(), isLink, test.link)
			}
		})
	}
}

func TestAttributesOnlyAndPreserveModeTimestamps(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantTime := time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := os.Chtimes(src, wantTime, wantTime); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--attributes-only", "--preserve=mode,timestamps", src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "keep" {
		t.Fatalf("attributes-only changed contents: %q", contents)
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && srcInfo.Mode().Perm() != dstInfo.Mode().Perm() {
		t.Fatalf("mode = %v, want %v", dstInfo.Mode().Perm(), srcInfo.Mode().Perm())
	}
	if !dstInfo.ModTime().Equal(srcInfo.ModTime()) {
		t.Fatalf("modtime = %v, want %v", dstInfo.ModTime(), srcInfo.ModTime())
	}
}

func TestLinkAndCopyModes(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	if err := os.WriteFile(src, []byte("link data"), 0o644); err != nil {
		t.Fatal(err)
	}

	hard := filepath.Join(root, "hard")
	if err := run([]string{"-l", src, hard}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	hardInfo, err := os.Stat(hard)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(srcInfo, hardInfo) {
		t.Fatal("-l did not create a hard link")
	}

	symbolic := filepath.Join(root, "symbolic")
	if err := run([]string{"-s", src, symbolic}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	linkInfo, err := os.Lstat(symbolic)
	if err != nil {
		t.Fatal(err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("-s did not create a symbolic link")
	}
}

func TestInteractiveForceDoesNotCancelPrompt(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"-i", "-f", src, dst}, &out, bufio.NewReader(strings.NewReader("n\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "overwrite") {
		t.Fatal("-f incorrectly suppressed the -i prompt")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("declined overwrite changed destination: %q", got)
	}
}

func TestInteractiveAndNoClobberUseLastOption(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		flags    []string
		want     string
		prompted bool
	}{
		{name: "interactive-after-no-clobber", flags: []string{"-n", "-i"}, want: "new", prompted: true},
		{name: "no-clobber-after-interactive", flags: []string{"-i", "-n"}, want: "old", prompted: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dst := filepath.Join(root, test.name)
			if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			args := append(append([]string{}, test.flags...), src, dst)
			if err := run(args, &output, bufio.NewReader(strings.NewReader("y\n"))); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != test.want {
				t.Fatalf("destination = %q, want %q", got, test.want)
			}
			if strings.Contains(output.String(), "overwrite") != test.prompted {
				t.Fatalf("prompt output=%q, prompted=%t", output.String(), test.prompted)
			}
		})
	}
}

func TestRemoveDestinationReplacesSymlink(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	target := filepath.Join(root, "target")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dst); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := run([]string{"--remove-destination", src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	dstInfo, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if dstInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("--remove-destination retained the destination symlink")
	}
	targetContents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(targetContents) != "old" {
		t.Fatalf("symlink target contents = %q, want unchanged", targetContents)
	}
}

func TestDanglingDestinationSymlinkRequiresPosixModeOrRemoval(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	missing := filepath.Join(root, "missing-target")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(missing, dst); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := run([]string{src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err == nil {
		t.Fatal("copy to dangling symlink unexpectedly succeeded")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dangling link target exists after refusal: %v", err)
	}

	t.Setenv("POSIXLY_CORRECT", "1")
	if err := run([]string{src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(missing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "data" {
		t.Fatalf("POSIX dangling-link target = %q, want data", got)
	}
}

func TestNoPreserveOverridesArchiveAttributes(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-a", "--no-preserve=mode", src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want existing destination mode 0600", info.Mode().Perm())
	}
}

func TestStripTrailingSlashesAndVersion(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceArg := src + string(filepath.Separator)
	if err := run([]string{"-R", "--strip-trailing-slashes", sourceArg, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "file")); err != nil {
		t.Fatalf("copy after stripping trailing slash failed: %v", err)
	}
	var version bytes.Buffer
	if err := run([]string{"--version"}, &version, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(version.String(), "cp (u-root)") {
		t.Fatalf("version output = %q", version.String())
	}
}

func TestHelpUsesStandardOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runWithCtx(context.Background(), []string{"--help"}, &stdout, &stderr, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Usage: cp") {
		t.Fatalf("help output missing from stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("help unexpectedly wrote to stderr: %q", stderr.String())
	}
}

func TestReflinkSparseAndDebugFlags(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	data := append(make([]byte, 128*1024), []byte("tail")...)
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"--reflink=auto", "--sparse=always", "--debug", src, dst}, &out, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("copy options changed contents")
	}
	if !strings.Contains(out.String(), "copied") && !strings.Contains(out.String(), "reflinked") {
		t.Fatalf("debug output missing copy strategy: %q", out.String())
	}
	if !strings.Contains(out.String(), " -> ") {
		t.Fatalf("--debug did not imply --verbose: %q", out.String())
	}
}

func TestKeepDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	realDest := filepath.Join(root, "real-destination")
	destLink := filepath.Join(root, "destination-link")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file"), []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(realDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDest, destLink); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := run([]string{"-RT", "--keep-directory-symlink", src, destLink}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(realDest, "file")); err != nil {
		t.Fatalf("copy did not follow destination directory symlink: %v", err)
	}
}

func TestInvalidConflictingOptions(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-l", "-s", src, filepath.Join(root, "dest")},
		{"-n", "--backup", src, filepath.Join(root, "dest")},
		{"-t", root, "-T", src},
	} {
		if err := run(args, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err == nil {
			t.Fatalf("run(%v) succeeded; want conflicting-option error", args)
		}
	}
}

func TestBackupShortOptionUsesSimpleSuffix(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VERSION_CONTROL", "existing")
	if err := run([]string{"-b", "-S.bak", src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("backup = %q, want old", got)
	}
}

func TestForceBackupAllowsCopyOntoSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--backup=simple", "--force", path, path}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("copied source contents = %q, want original", got)
	}
	backup, err := os.ReadFile(path + "~")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != "original" {
		t.Fatalf("backup contents = %q, want original", backup)
	}
}

func TestOneFileSystemReportsUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux device IDs support --one-file-system")
	}
	root := t.TempDir()
	src := filepath.Join(root, "source")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"-xR", src, filepath.Join(root, "copy")}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("")))
	if err == nil || !strings.Contains(err.Error(), "one-file-system") {
		t.Fatalf("error = %v, want explicit unsupported --one-file-system error", err)
	}
}

func TestUpdateNoneFailIsReported(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "destination")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"--update=none-fail", src, dst}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("")))
	if err == nil || errors.Is(err, cp.ErrSkip) {
		t.Fatalf("error = %v, want a reported skip error", err)
	}
}

func TestMultipleSourcesContinueAfterCopyError(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	src := filepath.Join(root, "valid-source")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{filepath.Join(root, "missing"), src, target}, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("")))
	if err == nil {
		t.Fatal("copy succeeded despite a missing source")
	}
	got, readErr := os.ReadFile(filepath.Join(target, filepath.Base(src)))
	if readErr != nil {
		t.Fatalf("later source was not copied after an earlier error: %v", readErr)
	}
	if string(got) != "valid" {
		t.Fatalf("copied contents = %q, want valid", got)
	}
}
