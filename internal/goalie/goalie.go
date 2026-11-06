// Package goalie is the command-line module and the only public seam of the goalie tool.
package goalie

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/config"
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
	control, build := file.Rules.Dup.Control, newDup
	if slices.Contains(opts.ruleNames, "crap") {
		control, build = file.Rules.Crap.Control, newCrap
	}
	mode := cmp.Or(control.Mode, config.ModeFail)
	if mode == config.ModeOff {
		if len(opts.ruleNames) == 0 {
			return exitClean
		}
		// A maintainer names a rule that is off to try its limit on real code, without failing a build.
		mode = config.ModeWarn
	}
	selected, err := build(file, opts.base)
	if err != nil {
		return fail(stderr, err)
	}
	found, err := selected.Run(dir, stdout, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	if found && mode == config.ModeFail {
		return exitFailed
	}
	return exitClean
}

type rule interface {
	Run(dir string, stdout, stderr io.Writer) (found bool, err error)
}

func newDup(file config.Settings, base string) (rule, error) {
	ruleSettings := file.Rules.Dup
	dup := check.NewDup("goalie")
	var err error
	if dup.Base, err = revision(ruleSettings.Scope, base, file); err != nil {
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

func newCrap(file config.Settings, base string) (rule, error) {
	ruleSettings := file.Rules.Crap
	crap := check.NewCrap("goalie")
	var err error
	if crap.Base, err = revision(ruleSettings.Scope, base, file); err != nil {
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

func revision(scope config.Scope, base string, file config.Settings) (string, error) {
	if scope == config.ScopeAll {
		return "", nil
	}
	result := cmp.Or(base, file.Base)
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
