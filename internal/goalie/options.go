package goalie

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/cliflags"
)

const usage = `goalie checks Go code against one set of rules.

Usage:
  goalie check [rule] [--base <rev>]

Rules:
  dup   Reports functions with the same structure as another function.
  crap  Reports functions that are complex and poorly tested. It runs your
        tests to get coverage.

With no rule name, goalie runs dup, unless dup is off. Name crap to run it. You
can name one rule at a time. A rule that you name runs even if it is off, and
then it reports as warn.

Flags:
  --base <rev>  Compare against this revision. This replaces base in .goalierc.
                A rule with scope changes needs a revision from one of them.
  --help        Print this help.

Settings file:
  goalie reads .goalierc from the current directory only. It holds one JSON
  object. The settings base, paths, skipTags, skipFiles, and shared go at the
  top level. Each rule has its own object in "rules":

    {"base": "origin/main", "rules": {"dup": {"mode": "warn"}}}

  mode   off: no output. warn: reports, and never fails. fail: reports, and
         fails. The default is fail.
  scope  changes: only changed code. all: the whole code base. The default is
         changes.

  The dup rule also takes threshold, minLines, minNodes, weakAnchors,
  weakLiterals, and includeTests. The crap rule also takes limit, sharedLimit,
  coverCommand, and coverProfiles. A limit of 0 doesn't turn a rule off.

Coverage:
  Set coverCommand or coverProfiles, not both. Each entry in coverProfiles is a
  path or a glob. goalie reads those files and runs no command. Otherwise it
  runs coverCommand through sh, from the current directory. It sets
  GOCRAP_COVER_DIR to an empty folder, and the command must write one or more
  text coverage profiles there. The default command is:
    ` + check.DefaultCoverCommand + `

Exit codes:
  0  No rule failed.
  1  A rule failed.
  2  Any error, including a missing revision, an unknown rule name, an unknown
     mode or scope, or a failed coverage command.
`

type options struct {
	base      string
	ruleNames []string
	help      bool
}

func parseCheck(args []string) (options, error) {
	opts := options{}
	flags := flag.NewFlagSet("goalie check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.Func("base", "", func(value string) error {
		// Empty would otherwise fall back to the file, so an unset CI variable would go unnoticed.
		if value == "" {
			return errors.New("must name a revision")
		}
		opts.base = value
		return nil
	})
	parsed, err := cliflags.Parse(flags, args)
	if err != nil {
		return opts, err
	}
	opts.help, opts.ruleNames = parsed.Help, parsed.Paths
	// A typing mistake must not pass as a clean result.
	for _, name := range opts.ruleNames {
		if name != "dup" && name != "crap" {
			return opts, fmt.Errorf("unknown rule %q", name)
		}
	}
	// Running only one of the named rules would report the other as clean.
	if len(opts.ruleNames) > 1 {
		return opts, errors.New("names more than one rule: run one rule at a time")
	}
	return opts, nil
}
