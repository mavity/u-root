// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package tr translates or deletes characters.
package tr

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"unicode"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

type cmd struct {
	stdin  io.Reader
	stdout io.Writer
	del    bool
	tr     *transformer
}

func command(in io.Reader, out io.Writer, args []string, del bool) (*cmd, error) {
	tr, err := parse(args, del)
	if err != nil {
		return nil, err
	}

	return &cmd{
		stdin:  in,
		stdout: out,
		del:    del,
		tr:     tr,
	}, nil
}

func (c *cmd) run() error {
	ioEnv := core.IO{
		Stdin:  c.stdin,
		Stdout: c.stdout,
	}
	return runTr(context.Background(), ioEnv, c.tr, false, nil)
}

const name = "tr"

var escapeChars = map[rune]rune{
	'\\': '\\',
	'a':  '\a',
	'b':  '\b',
	'f':  '\f',
	'n':  '\n',
	'r':  '\r',
	't':  '\t',
	'v':  '\v',
}

type Set string

const (
	ALPHA Set = "[:alpha:]"
	DIGIT Set = "[:digit:]"
	GRAPH Set = "[:graph:]"
	CNTRL Set = "[:cntrl:]"
	PUNCT Set = "[:punct:]"
	SPACE Set = "[:space:]"
	ALNUM Set = "[:alnum:]"
	LOWER Set = "[:lower:]"
	UPPER Set = "[:upper:]"
)

var sets = map[Set]func(r rune) bool{
	ALNUM: func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	},
	ALPHA: unicode.IsLetter,
	DIGIT: unicode.IsDigit,
	GRAPH: unicode.IsGraphic,
	CNTRL: unicode.IsControl,
	PUNCT: unicode.IsPunct,
	SPACE: unicode.IsSpace,
	LOWER: unicode.IsLower,
	UPPER: unicode.IsUpper,
}

type transformer struct {
	transform      func(r rune) rune
	delete         func(r rune) bool
	sourceContains func(rune) bool
	targetContains func(rune) bool
	complementRune rune
}

func setToRune(s Set, outRune rune) *transformer {
	check := sets[s]
	return &transformer{
		transform: func(r rune) rune {
			if check(r) {
				return outRune
			}
			return r
		},
	}
}

func lowerToUpper() *transformer {
	return &transformer{
		transform: unicode.ToUpper,
	}
}

func upperToLower() *transformer {
	return &transformer{
		transform: unicode.ToLower,
	}
}

func runesToRunes(in []rune, out ...rune) *transformer {
	convs := make(map[rune]rune)
	sources := make(map[rune]bool, len(in))
	targets := make(map[rune]bool, len(out))
	l := len(out)
	for i, r := range in {
		ind := min(i, l-1)
		convs[r] = out[ind]
		sources[r] = true
		targets[out[ind]] = true
	}
	return &transformer{
		transform: func(r rune) rune {
			if outRune, ok := convs[r]; ok {
				return outRune
			}
			return r
		},
		sourceContains: func(r rune) bool { return sources[r] },
		targetContains: func(r rune) bool { return targets[r] },
		complementRune: out[l-1],
	}
}

func expandRange(runes []rune) []rune {
	var res []rune
	for i := 0; i < len(runes); i++ {
		if i+2 < len(runes) && runes[i+1] == '-' && runes[i] <= runes[i+2] {
			for r := runes[i]; r <= runes[i+2]; r++ {
				res = append(res, r)
			}
			i += 2
		} else {
			res = append(res, runes[i])
		}
	}
	return res
}

func unescape(s Set) ([]rune, error) {
	var out []rune
	var escape bool
	for _, r := range s {
		if escape {
			v, ok := escapeChars[r]
			if !ok {
				return nil, fmt.Errorf("unknown escape sequence '\\%c'", r)
			}
			out = append(out, v)
			escape = false
			continue
		}
		if r == '\\' {
			escape = true
			continue
		}
		out = append(out, r)
	}
	return expandRange(out), nil
}

func parse(args []string, del bool) (*transformer, error) {
	narg := len(args)
	switch {
	case narg == 0 || (narg == 1 && !del):
		return nil, fmt.Errorf("missing operand")
	case narg > 1 && del:
		return nil, fmt.Errorf("extra operand after %q", args[0])
	case narg > 2:
		return nil, fmt.Errorf("extra operand after %q", args[1])
	}

	set1 := Set(args[0])
	arg1, err := unescape(set1)
	if err != nil {
		return nil, err
	}
	sourceContains := setContains(set1, arg1)
	if del {
		return &transformer{
			transform:      func(r rune) rune { return r },
			delete:         sourceContains,
			sourceContains: sourceContains,
		}, nil
	}

	set2 := Set(args[1])

	if set1 == LOWER && set2 == UPPER {
		tr := lowerToUpper()
		tr.sourceContains = sourceContains
		tr.targetContains = sets[UPPER]
		tr.complementRune = 'Z'
		return tr, nil
	}
	if set1 == UPPER && set2 == LOWER {
		tr := upperToLower()
		tr.sourceContains = sourceContains
		tr.targetContains = sets[LOWER]
		tr.complementRune = 'z'
		return tr, nil
	}

	if (set2 == LOWER || set2 == UPPER) && (set1 != LOWER && set1 != UPPER) ||
		(set1 == LOWER && set2 == LOWER) || (set1 == UPPER && set2 == UPPER) {
		return nil, fmt.Errorf("misaligned [:upper:] and/or [:lower:] construct")
	}

	if _, ok := sets[set2]; ok {
		return nil, fmt.Errorf(`the only character classes that may appear in SET2 are 'upper' and 'lower'`)
	}

	arg2, err := unescape(set2)
	if err != nil {
		return nil, err
	}
	if len(arg2) == 0 {
		return nil, fmt.Errorf("SET2 must be non-empty")
	}
	if _, ok := sets[set1]; ok {
		return &transformer{
			transform:      setToRune(set1, arg2[0]).transform,
			sourceContains: sourceContains,
			targetContains: runeSet(arg2),
			complementRune: arg2[len(arg2)-1],
		}, nil
	}
	tr := runesToRunes(arg1, arg2...)
	tr.sourceContains = sourceContains
	tr.targetContains = runeSet(arg2)
	return tr, nil
}

func setContains(set Set, runes []rune) func(rune) bool {
	if contains, ok := sets[set]; ok {
		return contains
	}
	allowed := make(map[rune]bool, len(runes))
	for _, r := range runes {
		allowed[r] = true
	}
	return func(r rune) bool { return allowed[r] }
}

func runeSet(runes []rune) func(rune) bool {
	allowed := make(map[rune]bool, len(runes))
	for _, r := range runes {
		allowed[r] = true
	}
	return func(r rune) bool { return allowed[r] }
}

func negatePredicate(predicate func(rune) bool) func(rune) bool {
	return func(r rune) bool { return !predicate(r) }
}

func complementTransformer(tr *transformer) {
	sourceContains := tr.sourceContains
	if tr.delete != nil {
		tr.delete = negatePredicate(sourceContains)
		return
	}
	transform := tr.transform
	fallback := tr.complementRune
	tr.transform = func(r rune) rune {
		if sourceContains(r) {
			return transform(r)
		}
		return fallback
	}
}

func runTr(ctx context.Context, ioEnv core.IO, tr *transformer, squeeze bool, sqSet func(rune) bool) (retErr error) {
	in := bufio.NewReader(ioEnv.Stdin)
	out := bufio.NewWriter(ioEnv.Stdout)
	defer func() {
		if err := out.Flush(); retErr == nil && err != nil {
			retErr = fmt.Errorf("write error: %w", err)
		}
	}()

	var prevRune rune
	hasPrev := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		inRune, size, err := in.ReadRune()
		if inRune == unicode.ReplacementChar && size == 1 {
			_ = in.UnreadRune()
			b, berr := in.ReadByte()
			if berr != nil {
				if berr == io.EOF {
					return nil
				}
				return fmt.Errorf("read error: %w", berr)
			}
			if tr.delete == nil || !tr.delete(inRune) {
				if werr := out.WriteByte(b); werr != nil {
					return fmt.Errorf("write error: %w", werr)
				}
			}
			hasPrev = false
		} else if size > 0 {
			if tr.delete != nil && tr.delete(inRune) {
				if err != nil {
					if err == io.EOF {
						return nil
					}
					return err
				}
				continue
			}
			outRune := tr.transform(inRune)
			if squeeze && hasPrev && outRune == prevRune && (sqSet == nil || sqSet(outRune)) {
				// duplicate squeezed
			} else {
				if _, werr := out.WriteRune(outRune); werr != nil {
					return fmt.Errorf("write error: %w", werr)
				}
				prevRune = outRune
				hasPrev = true
			}
		}

		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// Main runs the tr command.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "tr: internal panic: %v\n", r)
			exitCode = 1
		}
	}()

	var del, squeeze, complement bool
	fs := flag.NewFlagSet("tr", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	fs.BoolVar(&del, "delete", false, "delete characters in SET1, do not translate")
	fs.BoolVar(&del, "d", false, "delete characters in SET1, do not translate (shorthand)")
	fs.BoolVar(&squeeze, "squeeze-repeats", false, "squeeze repeated characters")
	fs.BoolVar(&squeeze, "s", false, "squeeze repeated characters (shorthand)")
	fs.BoolVar(&complement, "complement", false, "complement SET1")
	fs.BoolVar(&complement, "c", false, "complement SET1 (shorthand)")
	fs.BoolVar(&complement, "C", false, "complement SET1 (shorthand)")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	rest := fs.Args()
	if squeeze && !del && len(rest) == 1 {
		sqRunes, err := unescape(Set(rest[0]))
		if err != nil {
			fmt.Fprintf(ioEnv.Stderr, "tr: %v\n", err)
			return 1
		}
		sqSet := setContains(Set(rest[0]), sqRunes)
		if complement {
			sqSet = negatePredicate(sqSet)
		}
		tr := &transformer{transform: func(r rune) rune { return r }}
		if err := runTr(ctx, ioEnv, tr, true, sqSet); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return 130
			}
			fmt.Fprintf(ioEnv.Stderr, "tr: %v\n", err)
			return 1
		}
		return 0
	}

	tr, err := parse(rest, del)
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "tr: %v\n", err)
		return 1
	}

	var sqSet func(rune) bool
	if complement {
		complementTransformer(tr)
	}
	if squeeze {
		if del {
			sqSet = tr.sourceContains
			if complement {
				sqSet = negatePredicate(sqSet)
			}
		} else {
			sqSet = tr.targetContains
		}
	}

	if err := runTr(ctx, ioEnv, tr, squeeze, sqSet); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		fmt.Fprintf(ioEnv.Stderr, "tr: %v\n", err)
		return 1
	}
	return 0
}
