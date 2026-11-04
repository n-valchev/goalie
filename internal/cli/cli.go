// Package cli is the command-line module and the only public seam of godry.
package cli

import (
	"fmt"
	"io"

	"github.com/n-valchev/godry/internal/compare"
	"github.com/n-valchev/godry/internal/extract"
	"github.com/n-valchev/godry/internal/files"
	"github.com/n-valchev/godry/internal/fingerprint"
	"github.com/n-valchev/godry/internal/report"
)

const (
	exitClean      = 0
	exitDuplicates = 1
	exitError      = 2
)

// Run executes godry against dir and returns the process exit code.
func Run(args []string, dir string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args)
	if err != nil {
		return fail(stderr, err)
	}
	if opts.help {
		fmt.Fprint(stdout, usage)
		return exitClean
	}
	rules := files.Rules{IncludeTests: opts.includeTests, SkipTags: opts.skipTags}
	paths, err := files.Find(dir, opts.paths, rules)
	if err != nil {
		return fail(stderr, err)
	}
	if len(paths) == 0 {
		fmt.Fprintln(stdout, "No Go files found.")
		return exitClean
	}
	functions, err := extract.Parse(dir, paths)
	if err != nil {
		return fail(stderr, err)
	}
	pairs := compare.Find(candidates(functions, opts), nil, opts.threshold)
	report.Text(stdout, pairs)
	if len(pairs) > 0 && !opts.warnOnly {
		return exitDuplicates
	}
	return exitClean
}

func candidates(functions []extract.Function, opts options) []compare.Candidate {
	var result []compare.Candidate
	for _, fn := range functions {
		if fn.Lines() < opts.minLines {
			continue
		}
		set, nodes := fingerprint.Of(fn.Decl)
		if nodes < opts.minNodes {
			continue
		}
		result = append(result, compare.Candidate{Function: fn, Fingerprints: set, Nodes: nodes})
	}
	return result
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "godry: %v\n", err)
	return exitError
}
