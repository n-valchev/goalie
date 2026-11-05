package godeps

import (
	"flag"
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/cliflags"
	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/crap"
)

const usage = `godeps maps the packages of a Go module and the imports between them.

Usage:
  godeps [flags] [paths]

With no paths, godeps uses the paths in .goalierc, or else scans the current
directory tree. It reads go.mod from the current directory only, and leaves out
test files, the standard library, and imports of code it didn't scan.

Flags:
  --format F               Output format: html or json (default html).
  --arch-go-report <file>  Mark the edges that this arch-go report flags. Run
                           arch-go --json || true to write .arch-go/report.json.
  --crap-report <file>     Grade each function, class, and box from this gocrap
                           report, with the limits in .goalierc. A function that
                           the report leaves out is red. To score every
                           function, run gocrap --limit 0 --shared-limit 0
                           --warn-only --format json. Leave out --shared-limit
                           if you set no shared patterns.
  --dry-report <file>      Mark each function that this godry report pairs with
                           another. To make the report, run godry --format json
                           --warn-only.
  --editor-url <template>  Link to source with {file} and {line}
                           (default file://{file}).
  --title <text>           Title of the map (default: the module path).
  --skip-tags a,b          Skip files that can only build with these tags.
  --skip-files a,b         Skip files that match these patterns, with the rules
                           of godry --skip-files.
  --shared a,b             These patterns name your shared code.
  --help                   Print this help.

Settings file:
  godeps reads .goalierc from the current directory only. These settings at the
  top level apply to all goalie tools: paths, skipTags, skipFiles, shared. These
  settings go in the "godeps" object: format, archGoReport, crapReport,
  dryReport, editorURL, title, foreign, omit, order.

  foreign  Module paths outside this module. Each one that is imported shows as
           one class, which takes every import at or below that path.
  omit     Patterns that remove a class and its edges from the map. Each one is
           matched against the folder of the class, with the rules of
           --skip-files. gen/** covers the folder gen and all folders below it.
           gen alone covers any folder named gen, and nothing below it. Use .
           for a class at the repo root.
  order    Names of the top-level boxes, in the order to show them. The box of
           the shared code is named shared.

  A flag replaces the setting of the same name. Report paths start from the
  current directory, so run godeps and the tools that write the reports from the
  repo root.

  limit and sharedLimit in the "gocrap" object set the grades: green at or below
  half the limit, yellow at or below it, and red above it. godeps ignores the
  limit in the report.

  A base setting in the "gocrap" or "godry" object limits that report to new code.
  With gocrap, every other function then shows as red. With godry, duplicates in
  old code show no mark. Pass --base on the command line instead.

Exit codes:
  0  The output was written.
  2  Any error: a bad flag or setting, a Go file that doesn't parse, a missing
     go.mod, a report path that is set but missing, or a report that isn't valid
     JSON or lacks its findings.

godeps has no exit code 1, because it checks nothing. Warnings go to stderr and
do not change the exit code. The README documents the JSON model.
`

const (
	formatHTML = "html"
	formatJSON = "json"

	defaultEditorURL = "file://{file}"
)

type options struct {
	config.Repo
	format       string
	archGoReport string
	crapReport   string
	dryReport    string
	editorURL    string
	title        string
	foreign      []string
	omit         []string
	order        []string
	limits       crap.Limits
	help         bool
	fromFlags    map[string]bool
}

func parseOptions(args []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("godeps", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.StringVar(&opts.format, "format", formatHTML, "")
	flags.StringVar(&opts.archGoReport, "arch-go-report", "", "")
	flags.StringVar(&opts.crapReport, "crap-report", "", "")
	flags.StringVar(&opts.dryReport, "dry-report", "", "")
	flags.StringVar(&opts.editorURL, "editor-url", "", "")
	flags.StringVar(&opts.title, "title", "", "")
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
	if o.format != formatHTML && o.format != formatJSON {
		return fmt.Errorf("invalid --format %q: must be html or json", o.format)
	}
	return nil
}
