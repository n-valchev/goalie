package godry

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/cliflags"
)

const usage = `godry finds Go functions with the same structure.

Usage:
  godry [flags] [paths]

Modes:
  godry [paths]
      Whole repo: compare every function with every other function.
  godry --base <rev> [paths]
      New code: compare each new or changed function with every scanned
      function. Report only pairs with at least one new or changed function.

Paths limit both modes. With no paths, godry uses the paths in .goalierc, or
else scans the current directory tree.

Flags:
  --base <rev>     Turn on new-code mode against this revision.
  --threshold N    Minimum similarity score (default 0.82).
  --min-lines N    Minimum lines in a function (default 9).
  --min-nodes N    Minimum nodes in the normalized function (default 20).
  --weak-anchors N Drop findings with fewer than N anchors and low literals
                   (default 0, which drops nothing).
  --weak-literals X
                   Literal score below which a finding counts as low (default
                   0.5). A finding with - for literals is never dropped.
  --format F       Output format: text or json (default text).
  --warn-only      Exit with 0 when duplicates exist.
  --include-tests  Scan *_test.go files.
  --skip-tags a,b  Skip files that can only build with these tags.
  --skip-files a,b Skip files that match these patterns. A pattern with no
                   slash matches the file name in any folder. A pattern with a
                   slash matches the path from the current directory. * does
                   not cross folders. ** matches any number of folders.
                   Quote patterns so that the shell doesn't expand them.
                   Example: --skip-files 'packages.go,internal/generated/**'
  --shared a,b     Add a case to each finding. These patterns name your shared
                   code, with the rules of --skip-files. Every other first
                   folder counts as a product. Example: --shared 'pkg/**'
                   copies-shared    One function is shared, one is in a product.
                   across-products  The functions are in two products.
                   same-product     Both functions are in one product.
                   within-shared    Both functions are shared.
  --help           Print this help.

Skip rules, for both modes and for paths you name:
  - Directories: vendor, testdata, node_modules, and names that start with . or _.
  - *_test.go files, unless you set --include-tests.
  - Files with the header "Code generated ... DO NOT EDIT."
  - Files with an ignore build constraint.
  - Files that can only build with a tag in --skip-tags.
  - Files that match a pattern in --skip-files or in .goalieignore.
  - Pairs in one package with the same function name and the same receiver type.
  - Functions with //godry:ignore <reason> in the comment directly above them.
Other build constraints and GOOS or GOARCH file suffixes have no effect.

Ignore file:
  godry reads .goalieignore from the current directory only. Write one
  --skip-files pattern per line. Lines that start with # are comments.
  Both the file and the flag apply. ! and a trailing / are errors.

Settings file:
  godry reads .goalierc from the current directory only. It holds one JSON
  object. These settings at the top level apply to all goalie tools: paths,
  skipTags, skipFiles, shared. These settings go in the "godry" object: base,
  threshold, minLines, minNodes, weakAnchors, weakLiterals, format, warnOnly,
  includeTests. A flag or a path on the command line replaces the same
  setting. Patterns in skipFiles, --skip-files, and .goalieignore all apply.

Exit codes:
  0  No duplicates, or duplicates with --warn-only.
  1  Duplicates exist.
  2  Any error, even with --warn-only.
`

type options struct {
	check.Dup
	warnOnly  bool
	help      bool
	fromFlags map[string]bool
}

func parseOptions(args []string) (options, error) {
	opts := options{Dup: check.NewDup("godry")}
	flags := flag.NewFlagSet("godry", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.Func("base", "", func(value string) error {
		// Empty would otherwise mean whole-repo mode, so an unset CI variable would pass silently.
		if value == "" {
			return errors.New("must name a revision")
		}
		opts.Base = value
		return nil
	})
	flags.Float64Var(&opts.Threshold, "threshold", opts.Threshold, "")
	flags.IntVar(&opts.MinLines, "min-lines", opts.MinLines, "")
	flags.IntVar(&opts.MinNodes, "min-nodes", opts.MinNodes, "")
	flags.IntVar(&opts.WeakAnchors, "weak-anchors", opts.WeakAnchors, "")
	flags.Float64Var(&opts.WeakLiterals, "weak-literals", opts.WeakLiterals, "")
	flags.StringVar((*string)(&opts.Format), "format", string(opts.Format), "")
	flags.BoolVar(&opts.warnOnly, "warn-only", false, "")
	flags.BoolVar(&opts.IncludeTests, "include-tests", false, "")
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
	if !(o.Threshold >= 0 && o.Threshold <= 1) {
		return fmt.Errorf("invalid --threshold %v: must be between 0 and 1", o.Threshold)
	}
	if o.MinLines < 0 {
		return fmt.Errorf("invalid --min-lines %d: must not be negative", o.MinLines)
	}
	if o.MinNodes < 0 {
		return fmt.Errorf("invalid --min-nodes %d: must not be negative", o.MinNodes)
	}
	if o.WeakAnchors < 0 {
		return fmt.Errorf("invalid --weak-anchors %d: must not be negative", o.WeakAnchors)
	}
	if !(o.WeakLiterals >= 0 && o.WeakLiterals <= 1) {
		return fmt.Errorf("invalid --weak-literals %v: must be between 0 and 1", o.WeakLiterals)
	}
	return o.Format.Validate()
}
