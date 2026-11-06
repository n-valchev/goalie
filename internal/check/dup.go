// Package check holds the one implementation of each rule, for every command that runs it.
package check

import (
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/compare"
	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/extract"
	"github.com/n-valchev/goalie/internal/fingerprint"
	"github.com/n-valchev/goalie/internal/gitchange"
	"github.com/n-valchev/goalie/internal/report"
)

const ignoreDirective = "//godry:ignore"

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

func (f Format) Validate() error {
	if f != FormatText && f != FormatJSON {
		return fmt.Errorf("invalid --format %q: must be text or json", string(f))
	}
	return nil
}

func (f Format) write(w io.Writer, pairs []compare.Pair) error {
	if f == FormatJSON {
		return report.JSON(w, pairs)
	}
	return report.Text(w, pairs)
}

func (f Format) reportEmpty(message string, stdout, stderr io.Writer, writeEmpty func() error) error {
	if f == FormatJSON {
		// A reader of --format json must always be able to parse stdout.
		fmt.Fprintln(stderr, message)
		return writeEmpty()
	}
	fmt.Fprintln(stdout, message)
	return nil
}

// Dup finds functions with the same structure. An empty Base compares the whole repo.
type Dup struct {
	config.Repo
	Tool         string // Starts each line on stderr, so that a reader can see which command wrote it.
	Base         string
	Threshold    float64
	MinLines     int
	MinNodes     int
	WeakAnchors  int
	WeakLiterals float64
	Format       Format
	IncludeTests bool
}

// NewDup holds the default values, so that every command that runs the rule starts from the same limits.
func NewDup(tool string) Dup {
	return Dup{
		Tool:         tool,
		Threshold:    0.82,
		MinLines:     9,
		MinNodes:     20,
		WeakLiterals: 0.5,
		Format:       FormatText,
	}
}

func (d *Dup) ApplyRule(rule config.DupRule) {
	fill(&d.Threshold, rule.Threshold)
	fill(&d.MinLines, rule.MinLines)
	fill(&d.MinNodes, rule.MinNodes)
	fill(&d.WeakAnchors, rule.WeakAnchors)
	fill(&d.WeakLiterals, rule.WeakLiterals)
	fill(&d.IncludeTests, rule.IncludeTests)
}

func fill[T any](target, value *T) {
	if value != nil {
		*target = *value
	}
}

func (d Dup) Validate() error {
	// Written so that NaN, which fails every comparison, is rejected too.
	if !(d.Threshold >= 0 && d.Threshold <= 1) {
		return fmt.Errorf("invalid --threshold %v: must be between 0 and 1", d.Threshold)
	}
	if d.MinLines < 0 {
		return fmt.Errorf("invalid --min-lines %d: must not be negative", d.MinLines)
	}
	if d.MinNodes < 0 {
		return fmt.Errorf("invalid --min-nodes %d: must not be negative", d.MinNodes)
	}
	if d.WeakAnchors < 0 {
		return fmt.Errorf("invalid --weak-anchors %d: must not be negative", d.WeakAnchors)
	}
	if !(d.WeakLiterals >= 0 && d.WeakLiterals <= 1) {
		return fmt.Errorf("invalid --weak-literals %v: must be between 0 and 1", d.WeakLiterals)
	}
	return d.Format.Validate()
}

func (d Dup) Run(dir string, stdout, stderr io.Writer) (found bool, err error) {
	var changes gitchange.Changes
	if d.Base != "" {
		if changes, err = gitchange.Since(dir, d.Base); err != nil {
			return false, err
		}
	}
	paths, err := d.Find(dir, d.IncludeTests)
	if err != nil {
		return false, err
	}
	if len(paths) == 0 {
		return false, d.nothingToCompare("No Go files found.", stdout, stderr)
	}
	d.WarnUnmatched(stderr, d.Tool, paths)
	functions, err := extract.Parse(dir, paths, ignoreDirective)
	if err != nil {
		return false, err
	}
	all := d.candidates(functions)
	newFunctions := compare.WholeRepo()
	if d.Base != "" {
		newFunctions = findNew(all, changes.For(dir, paths))
		if newFunctions.Empty() {
			return false, d.nothingToCompare("No new functions found.", stdout, stderr)
		}
	}
	pairs, dropped := compare.DropWeak(
		compare.Find(all, newFunctions, d.Threshold),
		compare.WeakLimits{Anchors: d.WeakAnchors, Literals: d.WeakLiterals},
	)
	compare.Classify(pairs, d.Shared)
	if err := d.Format.write(stdout, pairs); err != nil {
		return false, err
	}
	// On stderr, so that stdout stays the same shape for anything that reads it.
	switch {
	case dropped == 1:
		fmt.Fprintf(stderr, "%s: 1 weak finding dropped\n", d.Tool)
	case dropped > 1:
		fmt.Fprintf(stderr, "%s: %d weak findings dropped\n", d.Tool, dropped)
	}
	return len(pairs) > 0, nil
}

func (d Dup) nothingToCompare(message string, stdout, stderr io.Writer) error {
	return d.Format.reportEmpty(message, stdout, stderr, func() error {
		return report.JSON(stdout, nil)
	})
}

func (d Dup) candidates(functions []extract.Function) []compare.Candidate {
	var result []compare.Candidate
	for _, fn := range functions {
		if fn.Lines() < d.MinLines {
			continue
		}
		fingerprints := fingerprint.Of(fn.Decl)
		if fingerprints.Nodes < d.MinNodes {
			continue
		}
		result = append(result, compare.Candidate{Function: fn, Result: fingerprints})
	}
	return result
}
