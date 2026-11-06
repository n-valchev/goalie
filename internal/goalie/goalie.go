// Package goalie is the command-line module and the only public seam of the goalie tool.
package goalie

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/files"
)

const (
	exitClean  = 0
	exitFailed = 1
	exitError  = 2
)

func Run(args []string, dir string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return fail(stderr, errors.New("needs a command: try goalie check"))
	}
	switch args[0] {
	case "check":
		return runCheck(args[1:], dir, stdout, stderr)
	case "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return exitClean
	}
	return fail(stderr, fmt.Errorf("unknown command %q", args[0]))
}

func runCheck(args []string, dir string, stdout, stderr io.Writer) int {
	opts, err := parseCheck(args)
	if err != nil {
		return fail(stderr, err)
	}
	if opts.help {
		fmt.Fprint(stdout, usage)
		return exitClean
	}
	file, err := config.Read(dir)
	if err != nil {
		return fail(stderr, err)
	}
	// All of them before any runs, so that a mistake in the settings doesn't wait for a test run.
	selected, err := selectRules(file, opts)
	if err != nil {
		return fail(stderr, err)
	}
	results, failures := runRules(selected, dir, stderr)
	// With no rule left to report, an empty list would read as clean code to anything that loads it.
	if failures > 0 && failures == len(selected) {
		return exitError
	}
	check.SortResults(results)
	if err := check.WriteResults(stdout, opts.format, results); err != nil {
		return fail(stderr, err)
	}
	if failures > 0 {
		return exitError
	}
	if slices.ContainsFunc(results, func(result check.Result) bool { return result.Mode == config.ModeFail }) {
		return exitFailed
	}
	return exitClean
}

type rule interface {
	Results(dir string, stderr io.Writer) ([]check.Result, error)
}

type ruleEntry struct {
	name    string
	control func(config.Rules) config.Control
	build   func(config.Settings, options) (rule, error)
}

var allRules = []ruleEntry{
	{check.DupRule, func(rules config.Rules) config.Control { return rules.Dup.Control }, newDup},
	{check.CrapRule, func(rules config.Rules) config.Control { return rules.Crap.Control }, newCrap},
}

type selectedRule struct {
	name  string
	mode  config.Mode
	check rule
}

func selectRules(file config.Settings, opts options) ([]selectedRule, error) {
	var selected []selectedRule
	for _, entry := range allRules {
		named := slices.Contains(opts.ruleNames, entry.name)
		if len(opts.ruleNames) > 0 && !named {
			continue
		}
		mode := cmp.Or(entry.control(file.Rules).Mode, config.ModeFail)
		if mode == config.ModeOff {
			if !named {
				continue
			}
			// A maintainer names a rule that is off to try its limit on real code, without failing a build.
			mode = config.ModeWarn
		}
		built, err := entry.build(file, opts)
		if err != nil {
			return nil, err
		}
		selected = append(selected, selectedRule{entry.name, mode, built})
	}
	return selected, nil
}

// runRules keeps going after an error, so that failed tests don't hide what the other rules found.
// The rules run at the same time, because a partial check of a large repo must stay under 1 second,
// and one after the other they come too close.
func runRules(selected []selectedRule, dir string, stderr io.Writer) (results []check.Result, failures int) {
	shared := &lockedWriter{target: stderr}
	perRule := make([][]check.Result, len(selected))
	errs := make([]error, len(selected))
	files.Parallel(len(selected), func(i int) {
		perRule[i], errs[i] = selected[i].check.Results(dir, shared)
	})
	for i, entry := range selected {
		if errs[i] != nil {
			fail(stderr, fmt.Errorf("%s: %w", entry.name, errs[i]))
			failures++
			continue
		}
		for _, result := range perRule[i] {
			result.Mode = cmp.Or(result.Mode, entry.mode)
			results = append(results, result)
		}
	}
	return results, failures
}

type lockedWriter struct {
	lock   sync.Mutex
	target io.Writer
}

func (w *lockedWriter) Write(text []byte) (int, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.target.Write(text)
}

func newDup(file config.Settings, opts options) (rule, error) {
	ruleSettings := file.Rules.Dup
	dup := check.NewDup("goalie")
	var err error
	if dup.Base, err = revision(ruleSettings.Scope, opts, file); err != nil {
		return nil, err
	}
	dup.ApplyRule(ruleSettings)
	if err := dup.Apply(file, nil); err != nil {
		return nil, err
	}
	if err := dup.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", config.Name, err)
	}
	return dup, nil
}

func newCrap(file config.Settings, opts options) (rule, error) {
	ruleSettings := file.Rules.Crap
	crap := check.NewCrap("goalie")
	crap.Partial = opts.partial
	var err error
	if crap.Base, err = revision(ruleSettings.Scope, opts, file); err != nil {
		return nil, err
	}
	crap.ApplyRule(ruleSettings)
	if err := crap.Apply(file, nil); err != nil {
		return nil, err
	}
	if err := cmp.Or(ruleSettings.Cover.Check(), crap.Limits.Check()); err != nil {
		return nil, fmt.Errorf("%s: %w", config.Name, err)
	}
	return crap, nil
}

func revision(scope config.Scope, opts options, file config.Settings) (string, error) {
	if opts.all || scope == config.ScopeAll {
		return "", nil
	}
	result := cmp.Or(opts.base, file.Base)
	// A default of HEAD would see no changes in CI, so a forgotten flag would pass every build.
	if result == "" {
		return "", fmt.Errorf("needs a revision: use --base <rev>, or set base in %s", config.Name)
	}
	return result, nil
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "goalie: %v\n", err)
	return exitError
}
