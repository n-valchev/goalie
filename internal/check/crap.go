package check

import (
	"cmp"
	"fmt"
	"io"
	"slices"

	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/coverage"
	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/extract"
)

const CrapRule = "crap"

// Crap finds functions that are complex and poorly tested.
type Crap struct {
	Scope
	Partial       bool
	Limits        crap.Limits
	CoverCommand  string
	CoverProfiles []string
}

func NewCrap(scope Scope) Crap {
	return Crap{
		Scope:        scope,
		Limits:       crap.Limits{Limit: crap.DefaultLimit},
		CoverCommand: DefaultCoverCommand,
	}
}

func (c *Crap) ApplyRule(rule config.CrapRule) {
	fill(&c.Limits.Limit, rule.Limit)
	fill(&c.CoverCommand, rule.CoverCommand)
	c.Limits.SharedLimit = rule.SharedLimit
	c.CoverProfiles = rule.CoverProfiles
}

// Apply also hands the shared patterns to the limits, which decide each function's limit from them.
func (c *Crap) Apply(file config.Settings, fromFlags map[string]bool) error {
	if err := c.Repo.Apply(file, fromFlags); err != nil {
		return err
	}
	c.Limits.Shared = c.Shared
	return nil
}

func (c Crap) Results(dir string, stderr io.Writer) ([]Result, error) {
	functions, notice, err := c.functions(dir, stderr, CrapRule)
	if err != nil {
		return nil, err
	}
	// Before the coverage step, so that a change with no functions doesn't run the tests.
	if notice != "" {
		noResults(stderr, CrapRule, notice)
		return nil, nil
	}
	if c.Partial {
		return c.estimates(functions), nil
	}
	findings, err := c.findings(dir, functions, stderr)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(findings))
	for _, found := range findings {
		results = append(results, Result{
			Rule: CrapRule, Value: found.Score, Limit: found.Limit,
			File: found.File, Line: found.Line, Message: found.message(), Details: found,
		})
	}
	return results, nil
}

func (c Crap) findings(dir string, functions []extract.Function, stderr io.Writer) ([]finding, error) {
	profile, err := c.gather(dir, stderr)
	if err != nil {
		return nil, err
	}
	return c.score(functions, profile), nil
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

func (c Crap) score(functions []extract.Function, profile coverage.Profile) []finding {
	findings := []finding{}
	for _, fn := range functions {
		complexity := crap.Complexity(fn.Decl)
		covered := profile.Of(fn.File, fn.StartLine, fn.EndLine)
		result, limit := crap.Score(complexity, covered), c.Limits.For(fn.File)
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

func (f finding) message() string {
	return fmt.Sprintf("%s scores %.1f (complexity %d, coverage %.1f%%, limit %g)",
		f.Function, f.Score, f.Complexity, f.Coverage*100, f.Limit)
}
