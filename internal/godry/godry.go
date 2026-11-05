// Package godry is the command-line module and the only public seam of the godry tool.
package godry

import (
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/compare"
	"github.com/n-valchev/goalie/internal/extract"
	"github.com/n-valchev/goalie/internal/fingerprint"
	"github.com/n-valchev/goalie/internal/gitchange"
	"github.com/n-valchev/goalie/internal/report"
)

const ignoreDirective = "//godry:ignore"

const (
	exitClean      = 0
	exitDuplicates = 1
	exitError      = 2
)

func Run(args []string, dir string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args)
	if err != nil {
		return fail(stderr, err)
	}
	if opts.help {
		fmt.Fprint(stdout, usage)
		return exitClean
	}
	if err := applyConfig(dir, &opts); err != nil {
		return fail(stderr, err)
	}
	var changes gitchange.Changes
	if opts.base != "" {
		if changes, err = gitchange.Since(dir, opts.base); err != nil {
			return fail(stderr, err)
		}
	}
	paths, err := opts.Find(dir, opts.includeTests)
	if err != nil {
		return fail(stderr, err)
	}
	if len(paths) == 0 {
		return nothingToCompare("No Go files found.", opts, stdout, stderr)
	}
	opts.WarnUnmatched(stderr, "godry", paths)
	functions, err := extract.Parse(dir, paths, ignoreDirective)
	if err != nil {
		return fail(stderr, err)
	}
	all := candidates(functions, opts)
	newFunctions := compare.WholeRepo()
	if opts.base != "" {
		newFunctions = findNew(all, changes.For(dir, paths))
		if newFunctions.Empty() {
			return nothingToCompare("No new functions found.", opts, stdout, stderr)
		}
	}
	pairs, dropped := compare.DropWeak(
		compare.Find(all, newFunctions, opts.threshold),
		compare.WeakLimits{Anchors: opts.weakAnchors, Literals: opts.weakLiterals},
	)
	compare.Classify(pairs, opts.Shared)
	if err := opts.format.write(stdout, pairs); err != nil {
		return fail(stderr, err)
	}
	// On stderr, so that stdout stays the same shape for anything that reads it.
	switch {
	case dropped == 1:
		fmt.Fprintln(stderr, "godry: 1 weak finding dropped")
	case dropped > 1:
		fmt.Fprintf(stderr, "godry: %d weak findings dropped\n", dropped)
	}
	if len(pairs) > 0 && !opts.warnOnly {
		return exitDuplicates
	}
	return exitClean
}

func nothingToCompare(message string, opts options, stdout, stderr io.Writer) int {
	if opts.format == formatJSON {
		// A reader of --format json must always be able to parse stdout.
		fmt.Fprintln(stderr, message)
		if err := report.JSON(stdout, nil); err != nil {
			return fail(stderr, err)
		}
		return exitClean
	}
	fmt.Fprintln(stdout, message)
	return exitClean
}

func candidates(functions []extract.Function, opts options) []compare.Candidate {
	var result []compare.Candidate
	for _, fn := range functions {
		if fn.Lines() < opts.minLines {
			continue
		}
		fingerprints := fingerprint.Of(fn.Decl)
		if fingerprints.Nodes < opts.minNodes {
			continue
		}
		result = append(result, compare.Candidate{Function: fn, Result: fingerprints})
	}
	return result
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "godry: %v\n", err)
	return exitError
}
