// Copyright 2012-2026 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// tsort writes to standard output a totally ordered list of items consistent
// with a partial ordering of items contained in the input. The standard input
// will be used if no file is specified.
//
// The input is a sequence of pairs of items, separated by <blank> characters.
// Pairs of different items (e.g., "a b") indicate ordering. Pairs of identical
// items (e.g., "c c") indicate presence, but not ordering.
//
// Synopsis:
//
//	tsort [FILE]
//
// Example:
//
//	tsort <<EOF
//	a b c c d e
//	g g
//	f g e f
//	h h
//	EOF
//
// produces an output like:
//
//	a
//	b
//	c
//	d
//	e
//	f
//	g
//	h
//
// which is one valid total ordering, but this is not guaranteed, it could
// equally be:
//
//	h
//	a
//	c
//	d
//	b
//	e
//	f
//	g
//
// or any other ordering where the following holds true:
//
//	- a is before b
//	- d is before e
//	- f is before g
//	- e is before f
//	- c is anywhere
//	- h is anywhere

package tsort

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

var (
	errNonFatal     = errors.New("non-fatal")
	errOddDataCount = errors.New("odd data count")
)

// Main runs the tsort command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "tsort panic: %v\n", r)
			exitCode = 1
		}
	}()

	fs := flag.NewFlagSet("tsort", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)
	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	rest := fs.Args()
	resolvedArgs := make([]string, len(rest))
	for i, a := range rest {
		if a != "-" {
			resolvedArgs[i] = ioEnv.ResolvePath(a)
		} else {
			resolvedArgs[i] = a
		}
	}

	err := runWithCtx(ctx, ioEnv.Stdin, ioEnv.Stdout, ioEnv.Stderr, resolvedArgs...)
	if errors.Is(err, errNonFatal) {
		return 1
	}
	if err != nil {
		fmt.Fprintf(ioEnv.Stderr, "tsort: %v\n", err)
		return 1
	}
	return 0
}

func run(stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	return runWithCtx(context.Background(), stdin, stdout, stderr, args...)
}

func runWithCtx(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	var err error
	in := io.NopCloser(stdin)
	if len(args) >= 1 && args[0] != "-" {
		f, openErr := os.Open(args[0])
		if openErr != nil {
			return openErr
		}
		in = f
	}
	defer in.Close()

	g := newGraph()
	if err = parseInto(ctx, in, g); err != nil {
		return err
	}

	topologicalOrdering(
		g,
		func(node nodeID) {
			_, _ = fmt.Fprintln(stdout, g.valueFor(node))
		},
		func(cycle []nodeID) {
			_, _ = fmt.Fprintln(stderr, "tsort: cycle in data")
			for _, node := range cycle {
				_, _ = fmt.Fprintln(stderr, "tsort:", g.valueFor(node))
			}
			err = errNonFatal
		})
	return err
}

func parseInto(ctx context.Context, in io.Reader, g *graph) error {
	scanner := bufio.NewScanner(in)
	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		a := scanner.Text()
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return errOddDataCount
		}

		b := scanner.Text()
		if a == b {
			g.addNode(a)
		} else {
			g.putEdge(a, b)
		}
	}

	return scanner.Err()
}

func topologicalOrdering(
	g *graph,
	f func(node nodeID),
	cycles func(cycle []nodeID),
) {
	// A topological ordering algorithm based on the depth-first search
	// algorithm in "Introduction to Algorithms" by Cormen et al.
	//
	// Unlike normal topological ordering, it returns an ordering even for
	// cyclic graphs by reporting any cycles found and pressing on as if the
	// cycles never existed.

	type visitState int8
	const (
		notVisited visitState = iota
		partiallyVisited
		fullyVisited
	)

	var path []nodeID
	result := make([]nodeID, 0, g.nodeCount())
	nodeToVisitState := make([]visitState, g.nodeCount())

	var doTopologicalOrdering func(node nodeID)
	doTopologicalOrdering = func(node nodeID) {
		nodeToVisitState[node] = partiallyVisited
		path = append(path, node)

		for _, succ := range g.successorIDs(node) {
			switch nodeToVisitState[succ] {
			case notVisited:
				doTopologicalOrdering(succ)
			case partiallyVisited:
				// Cycle detected
				idx := slices.Index(path, succ)
				cycle := path[idx:]
				cycles(cycle)
			case fullyVisited:
				continue
			}
		}

		path = path[:len(path)-1]
		nodeToVisitState[node] = fullyVisited

		result = append(result, node)
	}

	for node := range g.nodeIDs() {
		if nodeToVisitState[node] != fullyVisited {
			doTopologicalOrdering(node)
		}
	}

	for _, node := range slices.Backward(result) {
		f(node)
	}
}
