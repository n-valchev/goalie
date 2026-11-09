package check

import (
	"fmt"
	"io"
	"slices"
	"strings"
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
	for _, warning := range repo.UnmatchedWarnings(tool, paths) {
		w.warn(stderr, warning)
	}
}

func (w *Warnings) warn(stderr io.Writer, warning string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	if !w.printed[warning] {
		w.printed[warning] = true
		fmt.Fprint(stderr, warning)
	}
}

// changedFiles returns the Go files outside tests, and with a Base, only those that the change
// touches, with their changes. No other file can hold a result, and parsing all of them breaks the
// partial check's budget. changed is nil with no Base.
func (s Scope) changedFiles(dir string, stderr io.Writer) (paths []string, changed gitchange.Changes, err error) {
	var changes gitchange.Changes
	if s.Base != "" {
		if changes, err = s.Changes.Since(dir, s.Base); err != nil {
			return nil, nil, err
		}
	}
	if paths, err = s.Files.find(s.Repo, dir, false); err != nil {
		return nil, nil, err
	}
	if len(paths) == 0 {
		return nil, nil, nil
	}
	s.Warnings.warnUnmatched(stderr, s.Repo, paths)
	if s.Base == "" {
		return paths, nil, nil
	}
	changed = changes.For(dir, paths)
	paths = slices.DeleteFunc(paths, func(file string) bool {
		_, ok := changed[file]
		return !ok
	})
	return paths, changed, nil
}

// functions skips test files, which no function rule checks. A notice says why there is nothing
// to check.
func (s Scope) functions(dir string, stderr io.Writer, rule string) (functions []extract.Function, notice string, err error) {
	paths, changed, err := s.changedFiles(dir, stderr)
	if err != nil {
		return nil, "", err
	}
	if len(paths) == 0 && changed == nil {
		return nil, "No Go files found.", nil
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
	Of      func(extract.Function) rules.Measurement
	// Format takes the function name and the value, as in "%s has %d lines". When the measurement
	// names parameters, it takes the function name and the list of parameters instead.
	Format string
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
	ComplexCondition = Measure{
		Name: "complex-condition", DefaultLimit: 3, Of: rules.ComplexCondition,
		Format: "%s has a condition with %d operators",
	}
	CognitiveComplexity = Measure{
		Name: "cognitive-complexity", DefaultLimit: 15, Of: rules.CognitiveComplexity,
		Format: "%s has cognitive complexity %d",
	}
	PrimitiveArguments = Measure{
		Name: "primitive-arguments", DefaultLimit: 1, Of: rules.PrimitiveArguments, Format: "%s passes %s",
	}
)

// Limited is what every rule with one limit shares, so that a function rule and a file rule take
// their settings and flags in the same way.
type Limited struct {
	Scope
	Name  string
	Limit float64
}

func (l *Limited) ApplyRule(rule config.LimitRule) {
	fill(&l.Limit, rule.Limit)
}

func (l Limited) Validate() error {
	// Written so that NaN, which fails every comparison, is rejected too.
	if !(l.Limit >= 0) {
		return fmt.Errorf("invalid limit %v for %s: must not be negative", l.Limit, l.Name)
	}
	return nil
}

type Function struct {
	Limited
	Measure Measure
}

func NewFunction(scope Scope, measure Measure) *Function {
	return &Function{Limited: Limited{Scope: scope, Name: measure.Name, Limit: measure.DefaultLimit}, Measure: measure}
}

type measured struct {
	File       string      `json:"file"`
	Line       int         `json:"line"`
	Function   string      `json:"function"`
	Parameters []parameter `json:"parameters,omitempty"`
}

type parameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
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
		count := f.Measure.Of(fn)
		if float64(count.Value) <= f.Limit {
			continue
		}
		found := measured{File: fn.File, Line: count.Line, Function: name(fn)}
		message := fmt.Sprintf(f.Measure.Format+" (limit %g)", found.Function, count.Value, f.Limit)
		if len(count.Parameters) > 0 {
			var passed []string
			for _, param := range count.Parameters {
				found.Parameters = append(found.Parameters, parameter(param))
				passed = append(passed, param.Name+" down as "+param.Type)
			}
			message = fmt.Sprintf(f.Measure.Format+" (%d, limit %g)", found.Function, strings.Join(passed, ", "),
				count.Value, f.Limit)
		}
		results = append(results, Result{
			Rule: f.Measure.Name, Value: float64(count.Value), Limit: f.Limit, File: fn.File, Line: count.Line,
			Message: message, Details: found,
		})
	}
	return results, nil
}
