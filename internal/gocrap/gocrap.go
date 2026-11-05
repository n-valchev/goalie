// Package gocrap is the command-line module and the only public seam of the gocrap tool.
package gocrap

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/n-valchev/goalie/internal/coverage"
	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/extract"
	"github.com/n-valchev/goalie/internal/gitchange"
)

const ignoreDirective = "//gocrap:ignore"

const (
	exitClean = 0
	exitOver  = 1
	exitError = 2
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
	paths, err := opts.Find(dir, false)
	if err != nil {
		return fail(stderr, err)
	}
	if len(paths) == 0 {
		return nothingToScore("No Go files found.", opts, stdout, stderr)
	}
	opts.WarnUnmatched(stderr, "gocrap", paths)
	functions, err := extract.Parse(dir, paths, ignoreDirective)
	if err != nil {
		return fail(stderr, err)
	}
	if opts.base != "" {
		changed := changes.For(dir, paths)
		functions = slices.DeleteFunc(functions, func(fn extract.Function) bool {
			return !changed.Touches(fn.File, fn.StartLine, fn.EndLine)
		})
		// Before the coverage step, so that a change with no functions doesn't run the tests.
		if len(functions) == 0 {
			return nothingToScore("No new functions found.", opts, stdout, stderr)
		}
	}
	profile, err := gather(dir, opts, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	findings := score(functions, profile, opts)
	if err := writeReport(stdout, findings, opts.format); err != nil {
		return fail(stderr, err)
	}
	if len(findings) > 0 && !opts.warnOnly {
		return exitOver
	}
	return exitClean
}

type finding struct {
	File       string  `json:"file"`
	Line       int     `json:"line"`
	Function   string  `json:"function"`
	Score      float64 `json:"score"`
	Complexity int     `json:"complexity"`
	Coverage   float64 `json:"coverage"`
	Limit      float64 `json:"limit"`
}

func score(functions []extract.Function, profile coverage.Profile, opts options) []finding {
	findings := []finding{}
	for _, fn := range functions {
		complexity := crap.Complexity(fn.Decl)
		covered := profile.Of(fn.File, fn.StartLine, fn.EndLine)
		result, limit := crap.Score(complexity, covered), opts.limits.For(fn.File)
		if result > limit {
			findings = append(findings, finding{fn.File, fn.StartLine, name(fn), result, complexity, covered, limit})
		}
	}
	slices.SortStableFunc(findings, func(a, b finding) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	return findings
}

func name(fn extract.Function) string {
	if fn.Receiver != "" {
		return fn.Receiver + "." + fn.Name
	}
	return fn.Name
}

func writeReport(w io.Writer, findings []finding, format string) error {
	if format == formatJSON {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(struct {
			Findings []finding `json:"findings"`
		}{findings})
	}
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "No functions over the limit.")
		return err
	}
	for _, found := range findings {
		_, err := fmt.Fprintf(w, "%s:%d: %s scores %.1f (complexity %d, coverage %.1f%%, limit %g)\n",
			found.File, found.Line, found.Function, found.Score, found.Complexity, found.Coverage*100, found.Limit)
		if err != nil {
			return err
		}
	}
	return nil
}

func nothingToScore(message string, opts options, stdout, stderr io.Writer) int {
	if opts.format == formatJSON {
		// A reader of --format json must always be able to parse stdout.
		fmt.Fprintln(stderr, message)
		if err := writeReport(stdout, []finding{}, formatJSON); err != nil {
			return fail(stderr, err)
		}
		return exitClean
	}
	fmt.Fprintln(stdout, message)
	return exitClean
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "gocrap: %v\n", err)
	return exitError
}
