package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/n-valchev/godry/internal/compare"
	"github.com/n-valchev/godry/internal/files"
	"github.com/n-valchev/godry/internal/report"
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

Paths limit both modes. With no paths, godry scans the current directory tree.

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
  --help           Print this help.

Skip rules, for both modes and for paths you name:
  - Directories: vendor, testdata, node_modules, and names that start with . or _.
  - *_test.go files, unless you set --include-tests.
  - Files with the header "Code generated ... DO NOT EDIT."
  - Files with an ignore build constraint.
  - Files that can only build with a tag in --skip-tags.
  - Files that match a pattern in --skip-files or in .godryignore.
  - Pairs in one package with the same function name and the same receiver type.
  - Functions with //godry:ignore <reason> in the comment directly above them.
Other build constraints and GOOS or GOARCH file suffixes have no effect.

Ignore file:
  godry reads .godryignore from the current directory only. Write one
  --skip-files pattern per line. Lines that start with # are comments.
  Both the file and the flag apply. ! and a trailing / are errors.

Exit codes:
  0  No duplicates, or duplicates with --warn-only.
  1  Duplicates exist.
  2  Any error, even with --warn-only.
`

type format string

const (
	formatText format = "text"
	formatJSON format = "json"
)

func (f format) write(w io.Writer, pairs []compare.Pair) error {
	if f == formatJSON {
		return report.JSON(w, pairs)
	}
	return report.Text(w, pairs)
}

type options struct {
	base         string
	threshold    float64
	minLines     int
	minNodes     int
	format       format
	warnOnly     bool
	includeTests bool
	skipTags     []string
	skipFiles    []string
	weakAnchors  int
	weakLiterals float64
	help         bool
	paths        []string
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	flags := flag.NewFlagSet("godry", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.Func("base", "", func(value string) error {
		// Empty would otherwise mean whole-repo mode, so an unset CI variable would pass silently.
		if value == "" {
			return errors.New("must name a revision")
		}
		opts.base = value
		return nil
	})
	flags.Float64Var(&opts.threshold, "threshold", 0.82, "")
	flags.IntVar(&opts.minLines, "min-lines", 9, "")
	flags.IntVar(&opts.minNodes, "min-nodes", 20, "")
	flags.IntVar(&opts.weakAnchors, "weak-anchors", 0, "")
	flags.Float64Var(&opts.weakLiterals, "weak-literals", 0.5, "")
	flags.StringVar((*string)(&opts.format), "format", string(formatText), "")
	flags.BoolVar(&opts.warnOnly, "warn-only", false, "")
	flags.BoolVar(&opts.includeTests, "include-tests", false, "")
	flags.Func("skip-tags", "", func(value string) error {
		opts.skipTags = strings.Split(value, ",")
		return nil
	})
	flags.Func("skip-files", "", func(value string) error {
		for _, pattern := range strings.Split(value, ",") {
			cleaned, err := files.CleanPattern(pattern)
			if err != nil {
				return err
			}
			opts.skipFiles = append(opts.skipFiles, cleaned)
		}
		return nil
	})

	// The flag package stops at the first path, which would silently turn a later flag into a path.
	for rest := args; ; {
		err := flags.Parse(rest)
		if errors.Is(err, flag.ErrHelp) {
			opts.help = true
			return opts, nil
		}
		if err != nil {
			return opts, err
		}
		remaining := flags.Args()
		consumed := rest[:len(rest)-len(remaining)]
		if len(consumed) > 0 && consumed[len(consumed)-1] == "--" {
			opts.paths = append(opts.paths, remaining...)
			break
		}
		if len(remaining) == 0 {
			break
		}
		opts.paths = append(opts.paths, remaining[0])
		rest = remaining[1:]
	}
	return opts, opts.validate()
}

func (o options) validate() error {
	// Written so that NaN, which fails every comparison, is rejected too.
	if !(o.threshold >= 0 && o.threshold <= 1) {
		return fmt.Errorf("invalid --threshold %v: must be between 0 and 1", o.threshold)
	}
	if o.minLines < 0 {
		return fmt.Errorf("invalid --min-lines %d: must not be negative", o.minLines)
	}
	if o.minNodes < 0 {
		return fmt.Errorf("invalid --min-nodes %d: must not be negative", o.minNodes)
	}
	if o.weakAnchors < 0 {
		return fmt.Errorf("invalid --weak-anchors %d: must not be negative", o.weakAnchors)
	}
	if !(o.weakLiterals >= 0 && o.weakLiterals <= 1) {
		return fmt.Errorf("invalid --weak-literals %v: must be between 0 and 1", o.weakLiterals)
	}
	if o.format != formatText && o.format != formatJSON {
		return fmt.Errorf("invalid --format %q: must be text or json", o.format)
	}
	return nil
}
