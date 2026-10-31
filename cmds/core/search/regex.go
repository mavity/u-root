// Package search provides shared regular-expression matching for grep and rg.
package search

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"mohabbat/mohabbat/regast"
	"mohabbat/mohabbat/regast/syntax"
)

type Options struct {
	Basic      bool
	Fixed      bool
	IgnoreCase bool
	Perl       bool
	WholeWord  bool
	WholeLine  bool
}

type indexedRegexp interface {
	FindAllIndex([]byte, int) [][]int
}

type Matcher struct {
	regexps []indexedRegexp
	word    bool
}

func Compile(patterns []string, opts Options) (*Matcher, error) {
	m := &Matcher{word: opts.WholeWord}
	for _, pattern := range patterns {
		expr := pattern
		if !opts.Fixed && opts.Basic && !opts.Perl {
			translated, err := translateBRE(expr)
			if err != nil {
				return nil, fmt.Errorf("pattern %q: %w", pattern, err)
			}
			expr = translated
		}
		if opts.Fixed {
			expr = regexp.QuoteMeta(expr)
		}
		if opts.WholeWord {
			expr = `(?:` + expr + `)`
		}
		if opts.WholeLine {
			expr = `^(?:` + expr + `)$`
		}
		if opts.IgnoreCase {
			expr = `(?i)` + expr
		}

		var re indexedRegexp
		var err error
		if opts.Perl {
			re, err = regast.CompileWithFlags(expr, syntax.Perl|syntax.Backref)
		} else {
			re, err = regexp.Compile(expr)
		}
		if err != nil {
			return nil, fmt.Errorf("pattern %q: %w", pattern, err)
		}
		m.regexps = append(m.regexps, re)
	}
	if len(m.regexps) == 0 {
		return nil, fmt.Errorf("no patterns supplied")
	}
	return m, nil
}

func (m *Matcher) Match(record []byte) bool {
	return len(m.FindAllIndex(record, 1)) != 0
}

func (m *Matcher) FindAllIndex(record []byte, limit int) [][]int {
	if m == nil {
		return nil
	}
	var matches [][]int
	for _, re := range m.regexps {
		for _, match := range re.FindAllIndex(record, -1) {
			if len(match) < 2 || match[0] < 0 || match[1] < match[0] {
				continue
			}
			if m.word && (!IsWordBoundary(record, match[0]) || !IsWordBoundary(record, match[1])) {
				continue
			}
			matches = append(matches, match[:2])
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i][0] != matches[j][0] {
			return matches[i][0] < matches[j][0]
		}
		return matches[i][1] > matches[j][1]
	})
	out := matches[:0]
	lastStart, lastEnd := -1, -1
	for _, match := range matches {
		start, end := match[0], match[1]
		if start == lastStart && end == lastEnd {
			continue
		}
		if start < lastEnd {
			continue
		}
		out = append(out, match)
		lastStart, lastEnd = start, end
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func translateBRE(pattern string) (string, error) {
	var out strings.Builder
	inClass := false
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == '\\' && i+1 < len(pattern) {
			next := pattern[i+1]
			if inClass {
				out.WriteByte(ch)
				out.WriteByte(next)
				i++
				continue
			}
			switch next {
			case '(', ')', '{', '}':
				out.WriteByte(next)
			case '+', '?', '|':
				out.WriteByte(next)
			default:
				out.WriteByte(ch)
				out.WriteByte(next)
			}
			i++
			continue
		}
		if ch == '[' && !inClass {
			inClass = true
			out.WriteByte(ch)
			continue
		}
		if ch == ']' && inClass {
			inClass = false
			out.WriteByte(ch)
			continue
		}
		if !inClass && strings.ContainsRune("(){}+?|", rune(ch)) {
			out.WriteByte('\\')
		}
		out.WriteByte(ch)
	}
	if inClass {
		return "", fmt.Errorf("unterminated character class")
	}
	return out.String(), nil
}

func IsWordBoundary(record []byte, index int) bool {
	return !wordAt(record, index-1) != !wordAt(record, index)
}

func wordAt(record []byte, index int) bool {
	if index < 0 || index >= len(record) {
		return false
	}
	if record[index] == '_' {
		return true
	}
	if record[index]&0xc0 == 0x80 {
		for index > 0 && record[index]&0xc0 == 0x80 {
			index--
		}
	}
	r, _ := utf8.DecodeRune(record[index:])
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}

func FilterWholeWord(record []byte, matches [][]int) [][]int {
	out := matches[:0]
	for _, match := range matches {
		if IsWordBoundary(record, match[0]) && IsWordBoundary(record, match[1]) {
			out = append(out, match)
		}
	}
	return out
}

func FixedIndex(record, pattern []byte, from int, ignoreCase bool) int {
	if from < 0 || from > len(record) {
		return -1
	}
	if !ignoreCase {
		index := bytes.Index(record[from:], pattern)
		if index < 0 {
			return -1
		}
		return index + from
	}
	if len(pattern) == 0 {
		return from
	}
	want := string(pattern)
	for i := from; i+len(pattern) <= len(record); i++ {
		if strings.EqualFold(string(record[i:i+len(pattern)]), want) {
			return i
		}
	}
	return -1
}
