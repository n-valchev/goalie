package check

import (
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/extract"
	"github.com/n-valchev/goalie/internal/gitchange"
	"github.com/n-valchev/goalie/internal/rules"
)

// Scope says which code a rule checks. An empty Base checks the whole repo. Parser, Changes, and
// Warnings belong to the run, and every rule shares them.
type Scope struct {
	config.Repo
	Parser   *extract.Parser
	Changes  *gitchange.Cache
	Warnings *Warnings
	Base     string
}

// Warnings prints each line once, however many rules find it. Each rule still looks for itself,
// because dup reads test files and the others don't, so a pattern can match for one rule only.
type Warnings struct {
	lock    sync.Mutex
	printed map[string]bool
}

func NewWarnings() *Warnings {
	return &Warnings{printed: map[string]bool{}}
}

func (w *Warnings) warnUnmatched(stderr io.Writer, repo config.Repo, paths []string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	for _, warning := range repo.UnmatchedWarnings(tool, paths) {
		if !w.printed[warning] {
			w.printed[warning] = true
			fmt.Fprint(stderr, warning)
		}
	}
}

// functions skips test files, which no function rule checks. A notice says why there is nothing
// to check.
func (s Scope) functions(dir string, stderr io.Writer, rule string) (functions []extract.Function, notice string, err error) {
	var changes gitchange.Changes
	if s.Base != "" {
		if changes, err = s.Changes.Since(dir, s.Base); err != nil {
			return nil, "", err
		}
	}
	paths, err := s.Find(dir, false)
	if err != nil {
		return nil, "", err
	}
	if len(paths) == 0 {
		return nil, "No Go files found.", nil
	}
	s.Warnings.warnUnmatched(stderr, s.Repo, paths)
	var changed gitchange.Changes
	if s.Base != "" {
		changed = changes.For(dir, paths)
		// No other file can hold a result, and parsing all of them breaks the partial check's budget.
		paths = slices.DeleteFunc(paths, func(file string) bool {
			_, ok := changed[file]
			return !ok
		})
	}
	functions, err = s.Parser.Parse(dir, paths, rule)
	if err != nil {
		return nil, "", err
	}
	if s.Base != "" {
		functions = slices.DeleteFunc(functions, func(fn extract.Function) bool {
			return !changed.Touches(fn.File, fn.StartLine, fn.EndLine)
		})
		if len(functions) == 0 {
			return nil, "No new functions found.", nil
		}
	}
	return functions, "", nil
}

type Measure struct {
	Name         string
	DefaultLimit float64
	Of           func(extract.Function) int
	Format       string // Takes the function name and the value, as in "%s has %d lines".
}

var LongFunction = Measure{Name: "long-function", DefaultLimit: 80, Of: rules.Lines, Format: "%s has %d lines"}

type Function struct {
	Scope
	Measure Measure
	Limit   float64
}

func NewFunction(scope Scope, measure Measure) Function {
	return Function{Scope: scope, Measure: measure, Limit: measure.DefaultLimit}
}

func (f *Function) ApplyRule(rule config.FunctionRule) {
	fill(&f.Limit, rule.Limit)
}

func (f Function) Validate() error {
	// Written so that NaN, which fails every comparison, is rejected too.
	if !(f.Limit >= 0) {
		return fmt.Errorf("invalid limit %v for %s: must not be negative", f.Limit, f.Measure.Name)
	}
	return nil
}

type measured struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"function"`
}

// Results writes no notice when there is nothing to check, because with several function rules the
// same notice would repeat once for each of them.
func (f Function) Results(dir string, stderr io.Writer) ([]Result, error) {
	functions, _, err := f.functions(dir, stderr, f.Measure.Name)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, fn := range functions {
		value := f.Measure.Of(fn)
		if float64(value) <= f.Limit {
			continue
		}
		found := measured{fn.File, fn.StartLine, name(fn)}
		results = append(results, Result{
			Rule: f.Measure.Name, Value: float64(value), Limit: f.Limit, File: fn.File, Line: fn.StartLine,
			Message: fmt.Sprintf(f.Measure.Format+" (limit %g)", found.Function, value, f.Limit),
			Details: found,
		})
	}
	return results, nil
}
