package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

const usage = "Usage: godry [flags] [paths]\n"

type options struct {
	base         string
	threshold    float64
	minLines     int
	minNodes     int
	format       string
	warnOnly     bool
	includeTests bool
	skipTags     []string
	help         bool
	paths        []string
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	flags := flag.NewFlagSet("godry", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.StringVar(&opts.base, "base", "", "")
	flags.Float64Var(&opts.threshold, "threshold", 0.82, "")
	flags.IntVar(&opts.minLines, "min-lines", 9, "")
	flags.IntVar(&opts.minNodes, "min-nodes", 20, "")
	flags.StringVar(&opts.format, "format", "text", "")
	flags.BoolVar(&opts.warnOnly, "warn-only", false, "")
	flags.BoolVar(&opts.includeTests, "include-tests", false, "")
	flags.Func("skip-tags", "", func(value string) error {
		opts.skipTags = strings.Split(value, ",")
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
	if o.format != "text" && o.format != "json" {
		return fmt.Errorf("invalid --format %q: must be text or json", o.format)
	}
	return nil
}
