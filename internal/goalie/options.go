package goalie

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/cliflags"
)

const usage = `goalie checks Go code against one set of rules.

Usage:
  goalie check [rules] [--base <rev>] [--all] [--partial] [--format <format>]

Rules:
  dup   Reports functions with the same structure as another function.
  crap  Reports functions that are complex and poorly tested. It runs your
        tests to get coverage, unless you use --partial.

With no rule names, goalie runs every rule that isn't off. A rule that you name
runs even if it is off, and then it reports as warn.

Flags:
  --base <rev>       Compare against this revision. This replaces base in
                     .goalierc. A rule with scope changes needs a revision from
                     one of them.
  --all              Check the whole repository: every rule has scope all, and
                     no revision is needed. Each rule still follows its mode.
  --partial          Run no tests and read no coverage. See "Partial check".
  --format <format>  text or json. The default is text.
  --help             Print this help.

Results:
  Each line of text gives the file, the line, the mode, the rule, and the value
  and limit:

    a.go:3: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)

  fail results come first, then warn results. Within each, the highest value
  divided by its limit comes first. A limit of 0 counts as 1 for this order.

  stdout holds only results. With json, it holds a list called findings. Each
  entry has rule, mode, value, and limit, and the fields of its rule.

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

Partial check:
  The dup rule gives the same result as in a full check. The crap rule looks
  only at the complexity of each function:

  Safe            It passes even with no coverage. No output.
  Needs coverage  It reports the coverage that the function needs. This never
                  fails, in any mode, because only a test run can tell.
  Can't pass      Its complexity is over the limit, so no coverage can help.
                  This follows the mode of crap.

Exit codes:
  0  No fail result.
  1  At least one fail result.
  2  Any error, including a missing revision, an unknown rule name, an unknown
     mode or scope, or a failed coverage command. If the tests fail, goalie
     reports the other rules, skips crap, and prints the failure on stderr.
`

type options struct {
	base      string
	ruleNames []string
	all       bool
	partial   bool
	format    check.Format
	help      bool
}

func parseCheck(args []string) (options, error) {
	opts := options{format: check.FormatText}
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
	flags.Func("format", "", func(value string) error {
		opts.format = check.Format(value)
		return opts.format.Validate()
	})
	flags.BoolVar(&opts.all, "all", false, "")
	flags.BoolVar(&opts.partial, "partial", false, "")
	parsed, err := cliflags.Parse(flags, args)
	if err != nil {
		return opts, err
	}
	opts.help, opts.ruleNames = parsed.Help, parsed.Paths
	// A typing mistake must not pass as a clean result.
	for _, name := range opts.ruleNames {
		if !slices.ContainsFunc(allRules, func(entry ruleEntry) bool { return entry.name == name }) {
			return opts, fmt.Errorf("unknown rule %q", name)
		}
	}
	return opts, nil
}
