package check

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/coverage"
	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/extract"
	"github.com/n-valchev/goalie/internal/gitchange"
)

const CrapRule = "crap"

// Crap finds functions that are complex and poorly tested. An empty Base scores the whole repo.
type Crap struct {
	config.Repo
	Tool          string
	Partial       bool // Only Results reads this, because only goalie has a partial check.
	Parser        *extract.Parser
	Base          string
	Limits        crap.Limits
	CoverCommand  string
	CoverProfiles []string
	Format        Format
}

func NewCrap(tool string) Crap {
	return Crap{
		Tool:         tool,
		Parser:       extract.NewParser(extract.Directive{Name: crap.IgnoreDirective, Implied: CrapRule}),
		Limits:       crap.Limits{Limit: crap.DefaultLimit},
		CoverCommand: DefaultCoverCommand,
		Format:       FormatText,
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

func (c Crap) Run(dir string, stdout, stderr io.Writer) (found bool, err error) {
	functions, notice, err := c.functions(dir, stderr)
	if err != nil {
		return false, err
	}
	if notice != "" {
		return false, c.nothingToScore(notice, stdout, stderr)
	}
	findings, err := c.findings(dir, functions, stderr)
	if err != nil {
		return false, err
	}
	if err := c.write(stdout, findings); err != nil {
		return false, err
	}
	return len(findings) > 0, nil
}

func (c Crap) Results(dir string, stderr io.Writer) ([]Result, error) {
	functions, notice, err := c.functions(dir, stderr)
	if err != nil {
		return nil, err
	}
	if notice != "" {
		noResults(stderr, c.Tool, CrapRule, notice)
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

// A notice says why there is nothing to score.
func (c Crap) functions(dir string, stderr io.Writer) (functions []extract.Function, notice string, err error) {
	var changes gitchange.Changes
	if c.Base != "" {
		if changes, err = gitchange.Since(dir, c.Base); err != nil {
			return nil, "", err
		}
	}
	paths, err := c.Find(dir, false)
	if err != nil {
		return nil, "", err
	}
	if len(paths) == 0 {
		return nil, "No Go files found.", nil
	}
	c.WarnUnmatched(stderr, c.Tool, paths)
	var changed gitchange.Changes
	if c.Base != "" {
		changed = changes.For(dir, paths)
		// No other file can hold a result, and parsing all of them breaks the partial check's budget.
		paths = slices.DeleteFunc(paths, func(file string) bool {
			_, ok := changed[file]
			return !ok
		})
	}
	functions, err = c.Parser.Parse(dir, paths, CrapRule)
	if err != nil {
		return nil, "", err
	}
	if c.Base != "" {
		functions = slices.DeleteFunc(functions, func(fn extract.Function) bool {
			return !changed.Touches(fn.File, fn.StartLine, fn.EndLine)
		})
		// Before the coverage step, so that a change with no functions doesn't run the tests.
		if len(functions) == 0 {
			return nil, "No new functions found.", nil
		}
	}
	return functions, "", nil
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

func (c Crap) write(w io.Writer, findings []finding) error {
	if c.Format == FormatJSON {
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
		if _, err := fmt.Fprintf(w, "%s:%d: %s\n", found.File, found.Line, found.message()); err != nil {
			return err
		}
	}
	return nil
}

func (f finding) message() string {
	return fmt.Sprintf("%s scores %.1f (complexity %d, coverage %.1f%%, limit %g)",
		f.Function, f.Score, f.Complexity, f.Coverage*100, f.Limit)
}

func (c Crap) nothingToScore(message string, stdout, stderr io.Writer) error {
	return c.Format.reportEmpty(message, stdout, stderr, func() error {
		return c.write(stdout, []finding{})
	})
}
