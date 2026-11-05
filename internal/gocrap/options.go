package gocrap

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/n-valchev/goalie/internal/cliflags"
	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/crap"
)

const usage = `gocrap finds Go functions that are complex and poorly tested.

It scores each function with the CRAP formula, where CC is cyclomatic complexity:
  score = CC^2 x (1 - coverage)^3 + CC

Usage:
  gocrap [flags] [paths]

Modes:
  gocrap [paths]
      All code: score every function.
  gocrap --base <rev> [paths]
      New code: score only new or changed functions. Coverage still comes from
      all your test suites.

Paths limit both modes. With no paths, gocrap uses the paths in .goalierc, or
else scans the current directory tree.

Flags:
  --base <rev>       Turn on new-code mode against this revision.
  --limit N          Report functions that score above N (default 30). Use 0
                     to list every function.
  --shared-limit N   Use this limit for shared code, which needs --shared or
                     shared in .goalierc (default: the value of --limit).
  --cover-profiles a,b
                     Read these coverage profiles and run no command.
  --format F         Output format: text or json (default text).
  --warn-only        Exit with 0 when functions are over the limit.
  --skip-tags a,b    Skip files that can only build with these tags.
  --skip-files a,b   Skip files that match these patterns, with the rules of
                     godry --skip-files.
  --shared a,b       These patterns name your shared code.
  --help             Print this help.

Skip rules: the same as godry, and *_test.go files are never scored. Functions
with //gocrap:ignore <reason> in the comment directly above them are skipped.

Coverage:
  gocrap runs one command through sh, from the current directory. It sets
  GOCRAP_COVER_DIR to an empty folder, and the command must write one or more
  text coverage profiles there. gocrap merges them: a block counts as covered
  if any profile covers it. The default command is:
    ` + defaultCoverCommand + `
  To run several test suites, set coverCommand in .goalierc, for example to
  "make coverage". If the command fails, gocrap exits with 2.
  A function in no profile has 0% coverage. gocrap supports one Go module.

Settings file:
  gocrap reads .goalierc from the current directory only. These settings at the
  top level apply to all goalie tools: paths, skipTags, skipFiles, shared. These
  settings go in the "gocrap" object: base, limit, sharedLimit, coverCommand,
  coverProfiles, format, warnOnly. Set coverCommand or coverProfiles, not both.

Exit codes:
  0  No function is over its limit, or some are and you set --warn-only.
  1  A function is over its limit.
  2  Any error, even with --warn-only.
`

const (
	formatText = "text"
	formatJSON = "json"
)

type options struct {
	config.Repo
	base          string
	limit         float64
	sharedLimit   *float64
	coverCommand  string
	coverProfiles []string
	format        string
	warnOnly      bool
	help          bool
	fromFlags     map[string]bool
}

func parseOptions(args []string) (options, error) {
	opts := options{coverCommand: defaultCoverCommand}
	flags := flag.NewFlagSet("gocrap", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.Func("base", "", func(value string) error {
		// Empty would otherwise mean a scan of all code, so an unset CI variable would pass silently.
		if value == "" {
			return errors.New("must name a revision")
		}
		opts.base = value
		return nil
	})
	flags.Float64Var(&opts.limit, "limit", crap.DefaultLimit, "")
	flags.Func("shared-limit", "", func(value string) error {
		limit, err := strconv.ParseFloat(value, 64)
		opts.sharedLimit = &limit
		return err
	})
	flags.Func("cover-profiles", "", func(value string) error {
		opts.coverProfiles = strings.Split(value, ",")
		return nil
	})
	flags.StringVar(&opts.format, "format", formatText, "")
	flags.BoolVar(&opts.warnOnly, "warn-only", false, "")
	opts.AddFlags(flags)

	parsed, err := cliflags.Parse(flags, args)
	if err != nil {
		return opts, err
	}
	opts.Paths, opts.help, opts.fromFlags = parsed.Paths, parsed.Help, parsed.Set
	if opts.help {
		return opts, nil
	}
	return opts, opts.validate()
}

func (o options) validate() error {
	// Written so that NaN, which fails every comparison, is rejected too.
	if !(o.limit >= 0) {
		return fmt.Errorf("invalid --limit %v: must not be negative", o.limit)
	}
	if o.sharedLimit != nil && !(*o.sharedLimit >= 0) {
		return fmt.Errorf("invalid --shared-limit %v: must not be negative", *o.sharedLimit)
	}
	if o.format != formatText && o.format != formatJSON {
		return fmt.Errorf("invalid --format %q: must be text or json", o.format)
	}
	return nil
}
