package goalie

import (
	"flag"
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/cliflags"
)

const usage = `goalie checks Go code against one set of rules.

Usage:
  goalie check [rules] --base <rev>

Rules:
  dup  Reports new or changed functions with the same structure as another
       function. With no rule names, goalie runs every rule.

Flags:
  --base <rev>  Compare against this revision. The check needs one.
  --help        Print this help.

Exit codes:
  0  No rule failed.
  1  A rule failed.
  2  Any error, including a missing revision or an unknown rule name.
`

type options struct {
	base string
	help bool
}

func parseCheck(args []string) (options, error) {
	opts := options{}
	flags := flag.NewFlagSet("goalie check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.StringVar(&opts.base, "base", "", "")
	parsed, err := cliflags.Parse(flags, args)
	if err != nil {
		return opts, err
	}
	opts.help = parsed.Help
	rules := parsed.Paths
	// A typing mistake must not pass as a clean result.
	for _, name := range rules {
		if name != "dup" {
			return opts, fmt.Errorf("unknown rule %q", name)
		}
	}
	return opts, nil
}
