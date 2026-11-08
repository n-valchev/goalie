// Package check holds the implementation of each rule that goalie check runs.
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

const DupRule = "dup"

// Dup finds functions with the same structure.
type Dup struct {
	Scope
	Threshold    float64
	MinLines     int
	MinNodes     int
	WeakAnchors  int
	WeakLiterals float64
	IncludeTests bool
}

func NewDup(scope Scope) Dup {
	return Dup{
		Scope:        scope,
		Threshold:    0.82,
		MinLines:     9,
		MinNodes:     20,
		WeakLiterals: 0.5,
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
		return fmt.Errorf("invalid threshold %v: must be between 0 and 1", d.Threshold)
	}
	if d.MinLines < 0 {
		return fmt.Errorf("invalid minLines %d: must not be negative", d.MinLines)
	}
	if d.MinNodes < 0 {
		return fmt.Errorf("invalid minNodes %d: must not be negative", d.MinNodes)
	}
	if d.WeakAnchors < 0 {
		return fmt.Errorf("invalid weakAnchors %d: must not be negative", d.WeakAnchors)
	}
	if !(d.WeakLiterals >= 0 && d.WeakLiterals <= 1) {
		return fmt.Errorf("invalid weakLiterals %v: must be between 0 and 1", d.WeakLiterals)
	}
	return nil
}

func (d Dup) Results(dir string, stderr io.Writer) ([]Result, error) {
	pairs, notice, err := d.pairs(dir, stderr)
	if err != nil {
		return nil, err
	}
	if notice != "" {
		noResults(stderr, DupRule, notice)
		return nil, nil
	}
	results := make([]Result, 0, len(pairs))
	for _, pair := range pairs {
		results = append(results, Result{
			Rule: DupRule, Value: pair.Score, Limit: d.Threshold,
			File: pair.Left.File, Line: pair.Left.StartLine,
			Message: report.Summary(pair, d.Threshold), Details: report.Finding(pair),
		})
	}
	return results, nil
}

// A notice says why there is nothing to compare.
func (d Dup) pairs(dir string, stderr io.Writer) (pairs []compare.Pair, notice string, err error) {
	var changes gitchange.Changes
	if d.Base != "" {
		if changes, err = d.Changes.Since(dir, d.Base); err != nil {
			return nil, "", err
		}
	}
	paths, err := d.Files.find(d.Repo, dir, d.IncludeTests)
	if err != nil {
		return nil, "", err
	}
	if len(paths) == 0 {
		return nil, "No Go files found.", nil
	}
	d.Warnings.warnUnmatched(stderr, d.Repo, paths)
	functions, err := d.Parser.Parse(dir, paths, DupRule)
	if err != nil {
		return nil, "", err
	}
	all := d.candidates(functions)
	newFunctions := compare.WholeRepo()
	if d.Base != "" {
		newFunctions = findNew(all, changes.For(dir, paths))
		if newFunctions.Empty() {
			return nil, "No new functions found.", nil
		}
	}
	pairs, dropped := compare.DropWeak(
		compare.Find(all, newFunctions, d.Threshold),
		compare.WeakLimits{Anchors: d.WeakAnchors, Literals: d.WeakLiterals},
	)
	compare.Classify(pairs, d.Shared)
	// On stderr, so that stdout stays the same shape for anything that reads it.
	switch {
	case dropped == 1:
		fmt.Fprintf(stderr, "%s: 1 weak finding dropped\n", tool)
	case dropped > 1:
		fmt.Fprintf(stderr, "%s: %d weak findings dropped\n", tool, dropped)
	}
	return pairs, "", nil
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
