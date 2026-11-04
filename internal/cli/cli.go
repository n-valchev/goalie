// Package cli is the command-line module and the only public seam of godry.
package cli

import (
	"fmt"
	"io"

	"github.com/n-valchev/godry/internal/compare"
	"github.com/n-valchev/godry/internal/extract"
	"github.com/n-valchev/godry/internal/files"
	"github.com/n-valchev/godry/internal/fingerprint"
	"github.com/n-valchev/godry/internal/gitchange"
	"github.com/n-valchev/godry/internal/report"
)

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
	var changes gitchange.Changes
	if opts.base != "" {
		if changes, err = gitchange.Since(dir, opts.base); err != nil {
			return fail(stderr, err)
		}
	}
	ignored, err := files.ReadIgnoreFile(dir)
	if err != nil {
		return fail(stderr, err)
	}
	rules := files.Rules{
		IncludeTests: opts.includeTests,
		SkipTags:     opts.skipTags,
		SkipFiles:    append(ignored, opts.skipFiles...),
	}
	paths, err := files.Find(dir, opts.paths, rules)
	if err != nil {
		return fail(stderr, err)
	}
	if len(paths) == 0 {
		return nothingToCompare("No Go files found.", opts, stdout, stderr)
	}
	functions, err := extract.Parse(dir, paths)
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
	pairs := compare.Find(all, newFunctions, opts.threshold)
	if err := opts.format.write(stdout, pairs); err != nil {
		return fail(stderr, err)
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
