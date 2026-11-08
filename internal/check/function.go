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

// Scope says which code a rule checks. An empty Base checks the whole repo. Parser, Changes, Files,
// and Warnings belong to the run, and every rule shares them.
type Scope struct {
	config.Repo
	Parser   *extract.Parser
	Changes  *gitchange.Cache
	Files    *Files
	Warnings *Warnings
	Base     string
}

// Files finds the Go files once for each run. Reading the header of every file is most of the time
// of a partial check, and with each rule reading them again, all the rules go over the 1-second budget.
type Files struct {
	lock  sync.Mutex
	found map[filesKey]func() ([]string, error)
}

type filesKey struct {
	dir          string
	includeTests bool
}

func NewFiles() *Files {
	return &Files{found: map[filesKey]func() ([]string, error){}}
}

// find leaves repo out of the key, because every rule of a run reads the same file settings. It
// returns a copy, because a rule may delete from the list that it gets.
func (f *Files) find(repo config.Repo, dir string, includeTests bool) ([]string, error) {
	key := filesKey{dir, includeTests}
	f.lock.Lock()
	found, ok := f.found[key]
	if !ok {
		found = sync.OnceValues(func() ([]string, error) { return repo.Find(dir, includeTests) })
		f.found[key] = found
	}
	f.lock.Unlock()
	paths, err := found()
	return slices.Clone(paths), err
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
	paths, err := s.Files.find(s.Repo, dir, false)
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
	// Applies is nil for every function. It keeps the choice of functions out of Of, so that one
	// count serves two rules, as Parameters does for many-arguments and many-dependencies.
	Applies func(extract.Function) bool
	Of      func(extract.Function) int
	Format  string // Takes the function name and the value, as in "%s has %d lines".
}

var (
	LongFunction  = Measure{Name: "long-function", DefaultLimit: 80, Of: rules.Lines, Format: "%s has %d lines"}
	ManyArguments = Measure{
		Name: "many-arguments", DefaultLimit: 4, Applies: rules.IsNotConstructor,
		Of: rules.Parameters, Format: "%s has %d parameters",
	}
	ManyDependencies = Measure{
		Name: "many-dependencies", DefaultLimit: 5, Applies: rules.IsConstructor,
		Of: rules.Parameters, Format: "%s has %d parameters",
	}
)

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
		if f.Measure.Applies != nil && !f.Measure.Applies(fn) {
			continue
		}
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
