// Package godry is the command-line module and the only public seam of the godry tool.
package godry

import (
	"fmt"
	"io"
)

const (
	exitClean      = 0
	exitDuplicates = 1
	exitError      = 2
)

func Run(args []string, dir string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args)
	if err != nil {
		return fail(stderr, err)
	}
	if opts.help {
		fmt.Fprint(stdout, usage)
		return exitClean
	}
	if err := applyConfig(dir, &opts); err != nil {
		return fail(stderr, err)
	}
	found, err := opts.Dup.Run(dir, stdout, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	if found && !opts.warnOnly {
		return exitDuplicates
	}
	return exitClean
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "godry: %v\n", err)
	return exitError
}
