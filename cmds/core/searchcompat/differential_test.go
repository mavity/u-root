package searchcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	grepCmd "github.com/u-root/u-root/cmds/core/grep"
	rgCmd "github.com/u-root/u-root/cmds/core/rg"
	sedCmd "github.com/u-root/u-root/cmds/core/sed"
	"github.com/u-root/u-root/pkg/core"
)

type invocation struct {
	name                  string
	args                  []string
	input                 string
	files                 map[string][]byte
	observe               []string
	normalizeWindowsPaths bool
	normalizeJSON         bool
	normalizeStats        bool
	normalizeOutputOrder  bool
}

type result struct {
	stdout []byte
	stderr []byte
	status int
	files  map[string]fileSnapshot
}

type fileSnapshot struct {
	exists bool
	data   []byte
}

func TestGNUGrepDifferential(t *testing.T) {
	tests := []invocation{
		{name: "basic", args: []string{"needle"}, input: "needle\nother\n"},
		{name: "fixed-literal", args: []string{"-F", "a.b"}, input: "a.b\naxb\n"},
		{name: "invert-and-number", args: []string{"-v", "-n", "needle"}, input: "needle\nother\n"},
		{name: "only-matching", args: []string{"-o", "a+"}, input: "aa baa\n"},
		{name: "quiet-match", args: []string{"-q", "needle"}, input: "needle\n"},
		{name: "no-match-status", args: []string{"missing"}, input: "needle\n"},
		{
			name: "multiple-files-and-line-numbers",
			args: []string{"-n", "needle", "a.txt", "b.txt"},
			files: map[string][]byte{
				"a.txt": []byte("needle\nother\n"),
				"b.txt": []byte("other\nneedle\n"),
			},
		},
		{
			name:  "multiple-patterns",
			args:  []string{"-e", "alpha", "-e", "beta"},
			input: "alpha\nbeta\ngamma\n",
		},
		{
			name:  "pattern-file",
			args:  []string{"-f", "patterns.txt", "sample.txt"},
			files: map[string][]byte{"patterns.txt": []byte("alpha\nbeta\n"), "sample.txt": []byte("alpha\nbeta\ngamma\n")},
		},
		{
			name:  "empty-pattern-file",
			args:  []string{"-f", "patterns.txt", "sample.txt"},
			files: map[string][]byte{"patterns.txt": nil, "sample.txt": []byte("alpha\nbeta\n")},
		},
		{
			name:  "context",
			args:  []string{"-C", "1", "needle"},
			input: "before\nneedle\nafter\nlast\n",
		},
		{
			name:  "max-count",
			args:  []string{"-m", "1", "needle"},
			input: "needle\nneedle\nother\n",
		},
		{
			name: "counts-and-listed-files",
			args: []string{"-c", "needle", "a.txt", "b.txt"},
			files: map[string][]byte{
				"a.txt": []byte("needle\nneedle\n"),
				"b.txt": []byte("other\n"),
			},
		},
		{
			name:  "list-matches",
			args:  []string{"-l", "needle", "a.txt", "b.txt"},
			files: map[string][]byte{"a.txt": []byte("needle\n"), "b.txt": []byte("other\n")},
		},
		{
			name:  "list-without-matches",
			args:  []string{"-L", "needle", "a.txt", "b.txt"},
			files: map[string][]byte{"a.txt": []byte("needle\n"), "b.txt": []byte("other\n")},
		},
		{
			name:  "byte-offset",
			args:  []string{"-b", "needle"},
			input: "other\nneedle\n",
		},
		{
			name:  "no-final-newline",
			args:  []string{"-n", "needle"},
			input: "other\nneedle",
		},
		{
			name:  "pattern-starting-with-dash",
			args:  []string{"-e", "-needle"},
			input: "-needle\nother\n",
		},
		{
			name:  "empty-pattern",
			args:  []string{"-e", ""},
			input: "alpha\nbeta\n",
		},
		{
			name:  "extended-word-and-line-selection",
			args:  []string{"-E", "-w", "-x", "(alpha|beta)"},
			input: "alpha\nalpha beta\nxalpha\nbeta\n",
		},
		{
			name:  "perl-lookahead",
			args:  []string{"-P", "(?=needle)needle"},
			input: "needle\nother\n",
		},
		{
			name:  "perl-lookbehind",
			args:  []string{"-P", "(?<=prefix)needle"},
			input: "prefixneedle\nneedle\n",
		},
		{
			name:  "null-filename-separator",
			args:  []string{"-lZ", "needle", "sample.txt"},
			files: map[string][]byte{"sample.txt": []byte("needle\n")},
		},
		{
			name:  "binary-without-match",
			args:  []string{"-I", "needle"},
			input: "binary\x00needle\n",
		},
		{
			name:  "binary-as-text",
			args:  []string{"-a", "needle"},
			input: "binary\x00needle\n",
		},
		{
			name: "recursive-exclude-directory",
			args: []string{"-r", "--exclude-dir=skip", "needle", "."},
			files: map[string][]byte{
				"keep.txt":        []byte("needle\n"),
				"skip/hidden.txt": []byte("needle\n"),
			},
			normalizeWindowsPaths: true,
		},
		{
			name:                  "recursive-include-filter",
			args:                  []string{"-r", "--include=*.txt", "needle", "."},
			files:                 map[string][]byte{"keep.txt": []byte("needle\n"), "skip.go": []byte("needle\n"), "nested/keep.txt": []byte("needle\n")},
			normalizeWindowsPaths: true,
		},
		{name: "nul-delimited-records", args: []string{"-z", "needle"}, input: "other\x00needle\x00"},
	}
	runDifferential(t, "SEARCH_COMPAT_GNU_GREP", "grep", tests, grepMain)
}

func TestGNUSedDifferential(t *testing.T) {
	tests := []invocation{
		{name: "global-substitution", args: []string{"s/foo/bar/g"}, input: "foo foo\n"},
		{name: "line-selection", args: []string{"-n", "2p"}, input: "first\nsecond\nthird\n"},
		{name: "delete-range", args: []string{"1,2d"}, input: "first\nsecond\nthird\n"},
		{name: "quit-with-status", args: []string{"-n", "2Q 7"}, input: "first\nsecond\nthird\n"},
		{name: "extended-backreference", args: []string{"-E", "s/(foo)/[\\1]/g"}, input: "foo food\n"},
		{name: "address-range", args: []string{"1,/beta/d"}, input: "alpha\ninside\nbeta\nafter\n"},
		{name: "print-substitution-only", args: []string{"-n", "s/foo/bar/p"}, input: "foo\nother\n"},
		{name: "transliteration", args: []string{"y/abc/ABC/"}, input: "cab\n"},
		{name: "multiple-scripts", args: []string{"-e", "s/foo/bar/", "-e", "s/bar/baz/"}, input: "foo\n"},
		{name: "hold-space", args: []string{"1h; 2G"}, input: "first\nsecond\n"},
		{name: "multiline-pattern-space", args: []string{"N; s/first\\nsecond/combined/"}, input: "first\nsecond\nthird\n"},
		{name: "nul-delimited-records", args: []string{"-z", "s/foo/bar/"}, input: "foo\x00other\x00"},
		{name: "missing-final-newline", args: []string{"s/foo/bar/"}, input: "foo"},
		{
			name:  "separate-files-reset-addresses",
			args:  []string{"-s", "-n", "1p", "a.txt", "b.txt"},
			files: map[string][]byte{"a.txt": []byte("first\nsecond\n"), "b.txt": []byte("third\nfourth\n")},
		},
		{
			name:  "global-address-across-files",
			args:  []string{"-n", "1p", "a.txt", "b.txt"},
			files: map[string][]byte{"a.txt": []byte("first\nsecond\n"), "b.txt": []byte("third\nfourth\n")},
		},
		{
			name:    "write-command",
			args:    []string{"-n", "2w output.txt", "sample.txt"},
			files:   map[string][]byte{"sample.txt": []byte("first\nsecond\nthird\n")},
			observe: []string{"output.txt"},
		},
		{
			name:    "in-place-backup",
			args:    []string{"-i.bak", "s/foo/bar/g", "edit.txt"},
			files:   map[string][]byte{"edit.txt": []byte("foo foo\n")},
			observe: []string{"edit.txt", "edit.txt.bak"},
		},
		{
			name:  "script-file",
			args:  []string{"-f", "script.sed", "sample.txt"},
			files: map[string][]byte{"script.sed": []byte("s/foo/bar/g\n"), "sample.txt": []byte("foo foo\n")},
		},
	}
	runDifferential(t, "SEARCH_COMPAT_GNU_SED", "sed", tests, sedMain)
}

func TestRipgrepDifferential(t *testing.T) {
	tests := []invocation{
		{name: "stdin-default", args: []string{"needle"}, input: "needle\nother\n"},
		{name: "fixed-literal", args: []string{"-F", "a.b"}, input: "a.b\naxb\n"},
		{name: "line-number", args: []string{"-n", "needle"}, input: "other\nneedle\n"},
		{name: "only-matching", args: []string{"-o", "a+"}, input: "aa baa\n"},
		{name: "multiline-stdin", args: []string{"-U", "first\\nsecond"}, input: "first\nsecond\nlast\n"},
		{
			name:  "file-default-line-numbers",
			args:  []string{"needle", "sample.txt"},
			files: map[string][]byte{"sample.txt": []byte("other\nneedle\n")},
		},
		{
			name:  "file-no-line-numbers",
			args:  []string{"--no-line-number", "needle", "sample.txt"},
			files: map[string][]byte{"sample.txt": []byte("other\nneedle\n")},
		},
		{
			name:  "context",
			args:  []string{"-n", "-C", "1", "needle"},
			input: "before\nneedle\nafter\nlast\n",
		},
		{
			name:  "count-matching-lines",
			args:  []string{"--count", "needle", "sample.txt"},
			files: map[string][]byte{"sample.txt": []byte("needle\nneedle\nother\n")},
		},
		{
			name:  "count-matches",
			args:  []string{"--count-matches", "a+", "sample.txt"},
			files: map[string][]byte{"sample.txt": []byte("aa baa\n")},
		},
		{
			name:  "files-with-matches",
			args:  []string{"-l", "needle", "a.txt", "b.txt"},
			files: map[string][]byte{"a.txt": []byte("needle\n"), "b.txt": []byte("other\n")},
		},
		{
			name:  "files-without-match",
			args:  []string{"--files-without-match", "needle", "a.txt", "b.txt"},
			files: map[string][]byte{"a.txt": []byte("needle\n"), "b.txt": []byte("other\n")},
		},
		{
			name:  "max-count",
			args:  []string{"-m", "1", "-n", "needle"},
			input: "needle\nneedle\nother\n",
		},
		{
			name:  "pattern-file",
			args:  []string{"-f", "patterns.txt", "sample.txt"},
			files: map[string][]byte{"patterns.txt": []byte("alpha\nbeta\n"), "sample.txt": []byte("alpha\nbeta\ngamma\n")},
		},
		{
			name:  "pattern-starting-with-dash",
			args:  []string{"-e", "-needle"},
			input: "-needle\nother\n",
		},
		{
			name: "recursive-unrestricted",
			args: []string{"-uuu", "needle", "."},
			files: map[string][]byte{
				".hidden.txt": []byte("needle\n"),
				".gitignore":  []byte("ignored.txt\n"),
				"ignored.txt": []byte("needle\n"),
				"visible.txt": []byte("needle\n"),
			},
			normalizeWindowsPaths: true,
			normalizeOutputOrder:  true,
		},
		{
			name: "recursive-max-filesize",
			args: []string{"--max-filesize", "4", "needle", "."},
			files: map[string][]byte{
				"small.txt": []byte("needle\n"),
				"large.txt": []byte("needle and more\n"),
			},
			normalizeWindowsPaths: true,
		},
		{
			name:  "null-filename-separator",
			args:  []string{"-l", "--null", "needle", "a.txt", "b.txt"},
			files: map[string][]byte{"a.txt": []byte("needle\n"), "b.txt": []byte("other\n")},
		},
		{
			name: "files-listing",
			args: []string{"--files", "."},
			files: map[string][]byte{
				"a.txt":        []byte("needle\n"),
				"nested/b.txt": []byte("needle\n"),
				".hidden.txt":  []byte("needle\n"),
				".gitignore":   []byte("hidden\n"),
			},
			normalizeWindowsPaths: true,
			normalizeOutputOrder:  true,
		},
		{
			name: "recursive-ignore-and-hidden",
			args: []string{"needle", "."},
			files: map[string][]byte{
				".git/HEAD":       []byte("ref: refs/heads/main\n"),
				".gitignore":      []byte("ignored.txt\n"),
				".hidden.txt":     []byte("needle\n"),
				"ignored.txt":     []byte("needle\n"),
				"nested/keep.txt": []byte("needle\n"),
			},
			normalizeWindowsPaths: true,
		},
		{
			name: "recursive-glob-negation",
			args: []string{"--sort", "path", "-g", "!ignored.txt", "needle", "."},
			files: map[string][]byte{
				"keep.txt":       []byte("needle\n"),
				"ignored.txt":    []byte("needle\n"),
				"nested/keep.go": []byte("needle\n"),
			},
			normalizeWindowsPaths: true,
		},
		{
			name: "recursive-default-order",
			args: []string{"needle", "."},
			files: map[string][]byte{
				"top.txt":          []byte("needle\n"),
				"nested/second.go": []byte("needle\n"),
				"nested/first.go":  []byte("needle\n"),
			},
			normalizeWindowsPaths: true,
			normalizeOutputOrder:  true,
		},
		{
			name: "recursive-maxdepth",
			args: []string{"--maxdepth", "1", "needle", "."},
			files: map[string][]byte{
				"top.txt":         []byte("needle\n"),
				"nested/skip.txt": []byte("needle\n"),
			},
			normalizeWindowsPaths: true,
		},
		{
			name: "recursive-type",
			args: []string{"-t", "go", "needle", "."},
			files: map[string][]byte{
				"keep.go":     []byte("needle\n"),
				"skip.txt":    []byte("needle\n"),
				"nested/x.go": []byte("needle\n"),
			},
			normalizeWindowsPaths: true,
			normalizeOutputOrder:  true,
		},
		{
			name: "recursive-excluded-type",
			args: []string{"--type-not", "go", "needle", "."},
			files: map[string][]byte{
				"keep.txt":       []byte("needle\n"),
				"skip.go":        []byte("needle\n"),
				"nested/keep.md": []byte("needle\n"),
			},
			normalizeWindowsPaths: true,
			normalizeOutputOrder:  true,
		},
		{
			name: "case-insensitive-glob",
			args: []string{"--glob-case-insensitive", "-g", "*.TXT", "needle", "."},
			files: map[string][]byte{
				"keep.txt": []byte("needle\n"),
				"skip.go":  []byte("needle\n"),
			},
			normalizeWindowsPaths: true,
		},
		{
			name:  "nul-delimited-records",
			args:  []string{"--null-data", "needle"},
			input: "other\x00needle\x00",
		},
		{
			name:          "json-events",
			args:          []string{"--json", "needle", "sample.txt"},
			files:         map[string][]byte{"sample.txt": []byte("needle\n")},
			normalizeJSON: true,
		},
		{
			name:          "json-no-match",
			args:          []string{"--json", "missing", "sample.txt"},
			files:         map[string][]byte{"sample.txt": []byte("needle\n")},
			normalizeJSON: true,
		},
		{
			name:          "json-context",
			args:          []string{"--json", "-C", "1", "needle", "sample.txt"},
			files:         map[string][]byte{"sample.txt": []byte("before\nneedle\nafter\n")},
			normalizeJSON: true,
		},
		{
			name:           "stats",
			args:           []string{"--stats", "needle", "sample.txt"},
			files:          map[string][]byte{"sample.txt": []byte("needle\nother\n")},
			normalizeStats: true,
		},
	}
	runDifferential(t, "SEARCH_COMPAT_RG", "rg", tests, rgMain)
}

func runDifferential(t *testing.T, envName, defaultCommand string, tests []invocation, runLocal func(context.Context, core.IO, ...string) int) {
	t.Helper()
	command, ok := referenceCommand(t, envName, defaultCommand)
	if !ok {
		return
	}
	t.Logf("%s reference: %s", defaultCommand, referenceVersion(t, command, defaultCommand))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			local := runLocalResult(t, ctx, runLocal, tt)
			reference := runReference(t, ctx, command, tt)
			if runtime.GOOS == "windows" && tt.normalizeWindowsPaths {
				local.stdout = normalizeWindowsPathOutput(local.stdout)
				reference.stdout = normalizeWindowsPathOutput(reference.stdout)
			}
			if tt.normalizeOutputOrder {
				local.stdout = normalizeOutputOrder(local.stdout)
				reference.stdout = normalizeOutputOrder(reference.stdout)
			}
			if tt.normalizeJSON {
				local.stdout = normalizeJSONOutput(t, local.stdout)
				reference.stdout = normalizeJSONOutput(t, reference.stdout)
			}
			if tt.normalizeStats {
				local.stdout = normalizeStatsOutput(local.stdout)
				reference.stdout = normalizeStatsOutput(reference.stdout)
			}
			if local.status != reference.status ||
				!bytes.Equal(local.stdout, reference.stdout) ||
				!bytes.Equal(local.stderr, reference.stderr) ||
				!reflect.DeepEqual(local.files, reference.files) {
				t.Errorf(
					"behavior differs\nlocal:     status=%d stdout=%q stderr=%q files=%#v\nreference: status=%d stdout=%q stderr=%q files=%#v",
					local.status, local.stdout, local.stderr, local.files,
					reference.status, reference.stdout, reference.stderr, reference.files,
				)
			}
		})
	}
}

func normalizeWindowsPathOutput(output []byte) []byte {
	value := strings.ReplaceAll(string(output), ".\\", "./")
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.ReplaceAll(value, "./", "")
	return []byte(value)
}

func normalizeOutputOrder(output []byte) []byte {
	if len(output) == 0 {
		return output
	}
	terminated := output[len(output)-1] == '\n'
	lines := bytes.Split(bytes.TrimSuffix(output, []byte{'\n'}), []byte{'\n'})
	sort.Slice(lines, func(i, j int) bool {
		return bytes.Compare(lines[i], lines[j]) < 0
	})
	normalized := bytes.Join(lines, []byte{'\n'})
	if terminated {
		normalized = append(normalized, '\n')
	}
	return normalized
}

func normalizeJSONOutput(t *testing.T, output []byte) []byte {
	t.Helper()
	var normalized bytes.Buffer
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte{'\n'}) {
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("invalid JSON event %q: %v", line, err)
		}
		validateJSONElapsedFields(t, event)
		removeElapsedFields(event)
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("normalize JSON event: %v", err)
		}
		normalized.Write(encoded)
		normalized.WriteByte('\n')
	}
	return normalized.Bytes()
}

func validateJSONElapsedFields(t *testing.T, event map[string]any) {
	t.Helper()
	eventType, _ := event["type"].(string)
	data, _ := event["data"].(map[string]any)
	switch eventType {
	case "end":
		stats, _ := data["stats"].(map[string]any)
		assertElapsedValue(t, stats["elapsed"])
	case "summary":
		stats, _ := data["stats"].(map[string]any)
		assertElapsedValue(t, data["elapsed_total"])
		assertElapsedValue(t, stats["elapsed"])
	}
}

func assertElapsedValue(t *testing.T, value any) {
	t.Helper()
	elapsed, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("elapsed value has type %T, want object", value)
	}
	if _, ok := elapsed["secs"].(float64); !ok {
		t.Fatalf("elapsed secs has type %T, want number", elapsed["secs"])
	}
	if _, ok := elapsed["nanos"].(float64); !ok {
		t.Fatalf("elapsed nanos has type %T, want number", elapsed["nanos"])
	}
	human, ok := elapsed["human"].(string)
	if !ok {
		t.Fatalf("elapsed human has type %T, want string", elapsed["human"])
	}
	if _, err := time.ParseDuration(human); err != nil {
		t.Fatalf("invalid elapsed human duration %q: %v", human, err)
	}
}

func removeElapsedFields(value any) {
	switch item := value.(type) {
	case map[string]any:
		delete(item, "elapsed")
		delete(item, "elapsed_total")
		for _, child := range item {
			removeElapsedFields(child)
		}
	case []any:
		for _, child := range item {
			removeElapsedFields(child)
		}
	}
}

func normalizeStatsOutput(output []byte) []byte {
	var normalized []string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasSuffix(line, " seconds spent searching") || strings.HasSuffix(line, " seconds total") {
			continue
		}
		normalized = append(normalized, line)
	}
	return []byte(strings.Join(normalized, "\n"))
}

func runLocalResult(t *testing.T, ctx context.Context, main func(context.Context, core.IO, ...string) int, tt invocation) result {
	t.Helper()
	dir := makeFixture(t, tt.files)
	var stdout, stderr bytes.Buffer
	status := main(ctx, core.IO{
		Dir:             dir,
		Stdin:           strings.NewReader(tt.input),
		Stdout:          &stdout,
		Stderr:          &stderr,
		StdinIsTerminal: false,
	}, tt.args...)
	return result{
		stdout: stdout.Bytes(), stderr: stderr.Bytes(), status: status,
		files: snapshotFiles(t, dir, tt.observe),
	}
}

func runReference(t *testing.T, ctx context.Context, command []string, tt invocation) result {
	t.Helper()
	dir := makeFixture(t, tt.files)
	fullArgs := append(append([]string(nil), command[1:]...), tt.args...)
	cmd := exec.CommandContext(ctx, command[0], fullArgs...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(tt.input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return result{
			stdout: stdout.Bytes(), stderr: stderr.Bytes(),
			files: snapshotFiles(t, dir, tt.observe),
		}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return result{
			stdout: stdout.Bytes(), stderr: stderr.Bytes(), status: exitErr.ExitCode(),
			files: snapshotFiles(t, dir, tt.observe),
		}
	}
	t.Fatalf("run reference %q: %v", command[0], err)
	return result{}
}

func makeFixture(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := files[name]
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func snapshotFiles(t *testing.T, dir string, names []string) map[string]fileSnapshot {
	t.Helper()
	files := make(map[string]fileSnapshot, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		switch {
		case err == nil:
			files[name] = fileSnapshot{exists: true, data: data}
		case errors.Is(err, os.ErrNotExist):
			files[name] = fileSnapshot{}
		default:
			t.Fatalf("read fixture output %q: %v", name, err)
		}
	}
	return files
}

func referenceCommand(t *testing.T, envName, defaultCommand string) ([]string, bool) {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(envName))
	if value == "" {
		path, err := exec.LookPath(defaultCommand)
		if err == nil {
			return []string{path}, true
		}
		if os.Getenv("SEARCH_COMPAT_REQUIRE") == "1" {
			t.Fatalf("%s is unavailable; set %s to a JSON command array", defaultCommand, envName)
		}
		t.Skipf("%s is unavailable; set %s to a JSON command array to enable this differential corpus", defaultCommand, envName)
		return nil, false
	}

	var command []string
	if err := json.Unmarshal([]byte(value), &command); err != nil || len(command) == 0 || command[0] == "" {
		t.Fatalf("%s must be a JSON string array, for example [\"wsl.exe\",\"-e\",\"grep\"]: %v", envName, err)
	}
	if _, err := exec.LookPath(command[0]); err != nil {
		t.Fatalf("%s executable %q is unavailable: %v", envName, command[0], err)
	}
	return command, true
}

func referenceVersion(t *testing.T, command []string, tool string) string {
	t.Helper()
	args := append(append([]string(nil), command[1:]...), "--version")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, command[0], args...).CombinedOutput()
	if err != nil {
		t.Fatalf("read reference version: %v: %s", err, output)
	}
	output = bytes.ReplaceAll(output, []byte{0}, nil)
	expected := "GNU " + tool
	if tool == "rg" {
		expected = "ripgrep"
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(line), strings.ToLower(expected)) {
			return line
		}
	}
	t.Fatalf("reference version output did not contain %q: %q", expected, output)
	return ""
}

func grepMain(ctx context.Context, ioEnv core.IO, args ...string) int {
	return grepCmd.Main(ctx, ioEnv, args...)
}

func rgMain(ctx context.Context, ioEnv core.IO, args ...string) int {
	return rgCmd.Main(ctx, ioEnv, args...)
}

func sedMain(ctx context.Context, ioEnv core.IO, args ...string) int {
	return sedCmd.Main(ctx, ioEnv, args...)
}
