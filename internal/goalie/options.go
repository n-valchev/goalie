package goalie

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/cliflags"
)

const usage = `goalie checks Go code against one set of rules.

Usage:
  goalie check [rules] [--base <rev>] [--all] [--partial] [--format <format>]
  goalie check <rule> [--limit <n>] [--shared-limit <n>] [--warn-only] ...
  goalie serve

Rules:
  dup   Reports functions with the same structure as another function.
  crap  Reports functions that are complex and poorly tested. It runs your
        tests to get coverage, unless you use --partial. The score is
        CC^2 x (1 - coverage)^3 + CC, where CC is cyclomatic complexity.
  long-function
        Reports functions with more lines than the limit. The lines run from
        the func keyword to the closing brace, and blank lines and comments
        count.
  many-arguments
        Reports functions and methods with more parameters than the limit.
        Each name counts, so a, b int is 2. A receiver doesn't count.
  many-dependencies
        Reports constructors with more parameters than the limit. A
        constructor is a function, not a method, named New or New and a
        capital letter, such as NewServer. It uses this rule and not
        many-arguments.
  complex-condition
        Reports functions with a condition that has more && and || operators
        than the limit. A condition is the expression of an if, a for, or a
        case. A return or an assignment doesn't count. Each function reports
        once, at the line of its worst condition.
  cognitive-complexity
        Reports functions that are hard to follow. if, for, range, switch,
        and select each add 1, plus 1 for each block that they are inside.
        else adds 1. Code in a closure counts as 1 block deeper. && and ||
        add nothing. A guard adds 0: an if with no else, no inner block, a
        body that ends in return, continue, break, or panic, and one of
        these conditions:
          err != nil, or name != nil for a name that ends in Err
          !ok
        The condition may follow an init, as in if err := f(); err != nil.
  primitive-arguments
        Reports functions with a parameter of a basic type, such as string or
        int, that the function only passes down to calls into your repo. A
        call into an imported package or a built-in function, such as
        os.ReadFile(path) or len(id), and a conversion such as []byte(s), is
        a use, so the parameter doesn't report. An unused parameter doesn't
        report.
  long-file
        Reports files with more lines than the limit. Every line counts.
        With scope changes, a file reports only when the change takes it
        over the limit, or when the file is new. A moved or renamed file
        counts as new. It has no //goalie:ignore directive.
  high-mean-complexity
        Reports files whose functions have a mean complexity over the limit.
        Complexity is the one that crap uses. A function with no body
        doesn't count, and a file with no functions doesn't report. Scope
        changes works as for long-file. It has no //goalie:ignore directive.
  low-cohesion
        Reports struct types whose methods fall into more groups than the
        limit. Two methods are in one group when they use the same field of
        the receiver, or when one calls the other. Methods in every file of
        the package count, apart from test files. A method that uses no
        field and calls no method is left out. An accessor is left out too.
        An accessor only returns a receiver field, a field of that field, or
        the address of either, such as return s.conn. A call to an accessor
        counts as a use of its field. With scope changes, a type reports
        when the change touches its declaration or one of its methods. Write
        //goalie:ignore low-cohesion <reason> above the type.

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
  --limit <n>        Use this limit for the one rule that you name, in place of
                     .goalierc. The limit of dup is its threshold, from 0 to 1.
  --shared-limit <n> Use this sharedLimit for crap, in place of .goalierc.
  --warn-only        Run the one rule that you name in warn mode, so that its
                     results never exit with 1. An error still exits with 2.
  --help             Print this help.

Results:
  Each line of text gives the file, the line, the mode, the rule, and the value
  and limit:

    a.go:3: fail crap: Decide scores 42.0 (complexity 6, coverage 0.0%, limit 30)

  fail results come first, then warn results. Within each, the highest value
  divided by its limit comes first. A limit of 0 counts as 1 for this order.

  stdout holds only results. With json, it holds an object with a findings
  array. Each entry has rule, mode, value, and limit, and the fields of its
  rule.

Settings file:
  goalie reads .goalierc from the current directory only. It holds one JSON
  object. The settings base, paths, skipTags, skipFiles, and shared go at the
  top level. Each rule has its own object in "rules":

    {"base": "origin/main", "rules": {"dup": {"mode": "warn"}}}

  mode   off: no output. warn: reports, and never changes the exit code. fail:
         reports, and exits with 1. The default is fail for dup and crap, and
         warn for the other rules.
  scope  changes: only functions that the change touches, for long-file and
         high-mean-complexity, only files that the change takes over the
         limit, and for low-cohesion, only types whose declaration or methods
         the change touches. all: every function, file, or type in the whole
         repository. The default is changes.

  The dup rule also takes threshold (default 0.82), minLines (default 9),
  minNodes (default 20), weakAnchors (default 0), weakLiterals (default 0.5),
  and includeTests. A result with fewer anchors than weakAnchors and literals
  below weakLiterals is dropped, so the default drops nothing. A result whose
  literals are - is never dropped. The crap rule also takes limit (default 30),
  sharedLimit, coverCommand, and coverProfiles. The other rules also take
  limit: 80 for long-function, 4 for many-arguments, 5 for
  many-dependencies, 3 for complex-condition, 15 for
  cognitive-complexity, 1 for primitive-arguments, 1000 for long-file,
  7 for high-mean-complexity, and 1 for low-cohesion. A limit of 0 doesn't
  turn a rule off.

Files:
  goalie checks the Go files below the current directory, or in paths. It
  skips:
  - Folders named vendor, testdata, or node_modules, and names that start
    with . or _.
  - *_test.go files, unless dup sets includeTests. The other rules never
    check them.
  - Files with the header "Code generated ... DO NOT EDIT."
  - Files with an ignore build constraint, and files that can only build with
    a tag in skipTags. Other build constraints have no effect.
  - Files that match a pattern in skipFiles or in .goalieignore.
  dup also skips a pair in one package with the same function name and the
  same receiver type.

  A pattern with no slash matches the file name in any folder. A pattern with
  a slash matches the path from the current directory. * does not cross
  folders. ** matches any number of folders. .goalieignore holds one pattern
  per line, and goalie reads it from the current directory only. Lines that
  start with # are comments. ! and a trailing / are errors.

  The patterns in shared name your shared code. Every other first folder
  counts as a product. Each dup result then names its case:
    copies-shared    One function is shared, one is in a product.
    across-products  The functions are in two products.
    same-product     Both functions are in one product.
    within-shared    Both functions are shared.

Excuse a function:
  Write //goalie:ignore <rules> <reason> in the doc comment of a function. The
  rules that it names leave the function out, and the other rules still check
  it. Separate two rules with a comma and no space:

    //goalie:ignore crap,dup The table of cases needs every branch.

  An unknown rule name or a missing reason exits with 2. A dup directive also
  leaves out every pair with the function in it. For low-cohesion, write the
  directive directly above the type declaration.
  //godry:ignore and //gocrap:ignore have no effect.

Coverage:
  Set coverCommand or coverProfiles, not both. Each entry in coverProfiles is a
  path or a glob. goalie reads those files and runs no command. Otherwise it
  runs coverCommand through sh, from the current directory. It sets
  GOALIE_COVER_DIR to an empty folder, and the command must write one or more
  text coverage profiles there. The default command is:
    ` + check.DefaultCoverCommand + `
  A block counts as covered if any profile covers it. A function in no profile
  has 0% coverage. goalie supports one Go module.

Partial check:
  Every rule except crap gives the same result as in a full check. The crap
  rule looks only at the complexity of each function:

  Safe            It passes even with no coverage. No output.
  Needs coverage  It reports the coverage that the function needs. This never
                  fails, in any mode, because only a test run can tell.
  Can't pass      Its complexity is over the limit, so no coverage can help.
                  This follows the mode of crap.

Serve:
  goalie serve runs an MCP server for coding agents on stdin and stdout. Its
  tool check runs a partial check and gives up to 10 results. Each result
  says whether the change made it new, made it worse, or left it unchanged,
  and unchanged results come last. The tool takes dir, the absolute path of
  the folder that the agent works in, and reads the .goalierc nearest to it.
  With no base input and no base in .goalierc, it compares against HEAD. It skips a file that doesn't parse and
  lists it under unparsed. Logs go to stderr.

Exit codes:
  0  No fail result.
  1  At least one fail result.
  2  Any error, including a missing revision, an unknown rule name, an unknown
     mode or scope, --limit, --shared-limit, or --warn-only with no rule name
     or with several, or a failed coverage command. If the tests fail, goalie
     reports the other rules, skips crap, and prints the failure on stderr.
`

type options struct {
	base        string
	ruleNames   []string
	all         bool
	partial     bool
	format      check.Format
	limit       *float64
	sharedLimit *float64
	warnOnly    bool
	help        bool
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
	flags.Func("limit", "", limitFlag(&opts.limit))
	flags.Func("shared-limit", "", limitFlag(&opts.sharedLimit))
	flags.BoolVar(&opts.all, "all", false, "")
	flags.BoolVar(&opts.partial, "partial", false, "")
	flags.BoolVar(&opts.warnOnly, "warn-only", false, "")
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
	return opts, opts.checkOneRuleFlags(parsed.Set)
}

func limitFlag(target **float64) func(string) error {
	return func(value string) error {
		limit, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return errors.New("must be a number")
		}
		// Written so that NaN, which fails every comparison, is rejected too.
		if !(limit >= 0) {
			return errors.New("must not be negative")
		}
		*target = &limit
		return nil
	}
}

// checkOneRuleFlags allows these flags on one rule alone, because each rule reads its limit in its own
// way, and a limit for crap would make no sense as a threshold for dup.
func (o options) checkOneRuleFlags(set map[string]bool) error {
	for _, name := range []string{"limit", "shared-limit", "warn-only"} {
		if set[name] && len(o.ruleNames) != 1 {
			return fmt.Errorf("--%s needs exactly one rule name", name)
		}
	}
	if len(o.ruleNames) != 1 {
		return nil
	}
	if o.sharedLimit != nil && o.ruleNames[0] != check.CrapRule {
		return fmt.Errorf("--shared-limit works only with %s", check.CrapRule)
	}
	if o.ruleNames[0] == check.DupRule && o.limit != nil && *o.limit > 1 {
		return errors.New("--limit for dup must be between 0 and 1")
	}
	return nil
}
