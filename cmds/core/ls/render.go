// Copyright 2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lscmd

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

func (c cmd) colorEnabled() bool {
	switch c.colorMode {
	case "always":
		return true
	case "auto":
		if c.noColor {
			return false
		}
		if !c.terminal {
			return false
		}
		if c.env != nil && c.env("TERM") == "dumb" {
			return false
		}
		return true
	default:
		return false
	}
}

func (c cmd) autoEnabled(mode string) bool {
	return mode == "always" || mode == "auto" && c.terminal
}

func (c cmd) decorate(name string, f file) string {
	if c.autoEnabled(c.iconMode) {
		icon := fileIcon(f)
		if icon != "" {
			name = icon + " " + name
		}
	}
	if c.colorEnabled() {
		if code := c.colorCode(f); code != "" {
			name = "\x1b[" + code + "m" + name + "\x1b[0m"
		}
	}
	if c.autoEnabled(c.linkMode) {
		if target := fileURL(f.path); target != "" {
			name = "\x1b]8;;" + target + "\x1b\\" + name + "\x1b]8;;\x1b\\"
		}
	}
	return name
}

func fileIcon(f file) string {
	if f.info == nil {
		return ""
	}
	switch {
	case f.info.Mode()&os.ModeSymlink != 0:
		return "🔗"
	case f.info.IsDir():
		return "📁"
	case f.info.Mode()&os.ModeNamedPipe != 0:
		return "↔"
	case f.info.Mode()&os.ModeSocket != 0:
		return "◉"
	case f.info.Mode().IsRegular():
		switch strings.ToLower(filepath.Ext(f.name)) {
		case ".go", ".rs", ".c", ".h", ".py", ".js", ".ts":
			return "🧩"
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
			return "🖼"
		case ".zip", ".gz", ".bz2", ".xz", ".tar":
			return "📦"
		case ".md", ".txt", ".log":
			return "📄"
		}
		if f.info.Mode()&0o111 != 0 {
			return "⚙"
		}
		return "📄"
	default:
		return ""
	}
}

func fileURL(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
}

func (c cmd) colorCode(f file) string {
	if f.info == nil {
		return ""
	}
	if c.env != nil {
		if colors := c.env("LS_COLORS"); colors != "" {
			if code := colorFromLSColors(colors, f); code != "" {
				return code
			}
		}
	}
	switch {
	case f.info.Mode()&os.ModeSymlink != 0:
		return "36"
	case f.info.IsDir():
		return "1;34"
	case f.info.Mode()&os.ModeNamedPipe != 0:
		return "33"
	case f.info.Mode()&os.ModeSocket != 0:
		return "1;35"
	case f.info.Mode()&os.ModeDevice != 0:
		return "1;33"
	case f.info.Mode().IsRegular() && f.info.Mode()&0o111 != 0:
		return "1;32"
	default:
		return ""
	}
}

func colorFromLSColors(colors string, f file) string {
	for _, item := range strings.Split(colors, ":") {
		key, value, ok := strings.Cut(item, "=")
		if !ok || value == "" {
			continue
		}
		match := false
		switch key {
		case "di":
			match = f.info.IsDir()
		case "ln":
			match = f.info.Mode()&os.ModeSymlink != 0
		case "ex":
			match = f.info.Mode().IsRegular() && f.info.Mode()&0o111 != 0
		case "pi":
			match = f.info.Mode()&os.ModeNamedPipe != 0
		case "so":
			match = f.info.Mode()&os.ModeSocket != 0
		case "bd", "cd":
			match = f.info.Mode()&os.ModeDevice != 0
		default:
			if strings.HasPrefix(key, "*") {
				match, _ = filepath.Match(key, f.name)
			}
		}
		if match {
			return value
		}
	}
	return ""
}

func humanBlockSize(blocks int64, kib bool) string {
	size := blocks * 512
	if kib {
		size = blocks * 1024
	}
	for _, suffix := range []string{"B", "K", "M", "G", "T", "P"} {
		if size < 1024 || suffix == "P" {
			return fmt.Sprintf("%d%s", size, suffix)
		}
		size = (size + 512) / 1024
	}
	return fmt.Sprintf("%d", size)
}

func writeColumns(w io.Writer, entries []string, width int, across bool) {
	if len(entries) == 0 {
		return
	}
	if width <= 0 {
		width = 80
	}
	maxEntryWidth := 0
	entryWidths := make([]int, len(entries))
	for i, entry := range entries {
		entryWidths[i] = displayWidth(entry)
		if entryWidths[i] > maxEntryWidth {
			maxEntryWidth = entryWidths[i]
		}
	}
	cols := (width + 2) / (maxEntryWidth + 2)
	if cols < 1 {
		cols = 1
	}
	if cols > len(entries) {
		cols = len(entries)
	}
	rows := (len(entries) + cols - 1) / cols
	colWidths := make([]int, cols)
	for i, n := range entryWidths {
		col := i / rows
		if across {
			col = i % cols
		}
		if n > colWidths[col] {
			colWidths[col] = n
		}
	}
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			index := row + col*rows
			if across {
				index = row*cols + col
			}
			if index >= len(entries) {
				continue
			}
			fmt.Fprint(w, entries[index])
			nextIndex := row + (col+1)*rows
			if across {
				nextIndex = row*cols + col + 1
			}
			if col < cols-1 && nextIndex < len(entries) {
				padding := colWidths[col] - entryWidths[index] + 2
				if padding < 2 {
					padding = 2
				}
				fmt.Fprint(w, strings.Repeat(" ", padding))
			}
		}
		fmt.Fprintln(w)
	}
}

func displayWidth(s string) int {
	width := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			i = skipEscape(s, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			width++
		} else {
			width += runeWidth(r)
		}
		i += size
	}
	return width
}

func skipEscape(s string, i int) int {
	if i+1 >= len(s) {
		return i + 1
	}
	switch s[i+1] {
	case '[':
		i += 2
		for i < len(s) {
			b := s[i]
			i++
			if b >= 0x40 && b <= 0x7e {
				return i
			}
		}
	case ']':
		i += 2
		for i < len(s) {
			if s[i] == '\a' {
				return i + 1
			}
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
			i++
		}
	default:
		return i + 2
	}
	return len(s)
}

func runeWidth(r rune) int {
	if r == 0 || unicode.IsControl(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == '\u200d' {
		return 0
	}
	if r >= 0x1100 && (r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		r >= 0x2e80 && r <= 0xa4cf ||
		r >= 0xac00 && r <= 0xd7a3 ||
		r >= 0xf900 && r <= 0xfaff ||
		r >= 0xfe10 && r <= 0xfe19 ||
		r >= 0xfe30 && r <= 0xfe6f ||
		r >= 0xff00 && r <= 0xff60 ||
		r >= 0xffe0 && r <= 0xffe6 ||
		r >= 0x1f300 && r <= 0x1faff) {
		return 2
	}
	return 1
}
