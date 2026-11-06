// Package goalie is the command-line module and the only public seam of the goalie tool.
package goalie

import (
	"errors"
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/config"
)

const (
	exitClean  = 0
	exitFailed = 1
	exitError  = 2
)

func Run(args []string, dir string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return fail(stderr, errors.New("needs a command: try goalie check"))
	}
	switch args[0] {
	case "check":
		return runCheck(args[1:], dir, stdout, stderr)
	case "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return exitClean
	}
	return fail(stderr, fmt.Errorf("unknown command %q", args[0]))
}

func runCheck(args []string, dir string, stdout, stderr io.Writer) int {
	opts, err := parseCheck(args)
	if err != nil {
		return fail(stderr, err)
	}
	if opts.help {
		fmt.Fprint(stdout, usage)
		return exitClean
	}
	// A default of HEAD would see no changes in CI, so a forgotten flag would pass every build.
	if opts.base == "" {
		return fail(stderr, errors.New("needs a revision: use --base <rev>"))
	}
	file, err := config.Read(dir)
	if err != nil {
		return fail(stderr, err)
	}
	dup := check.NewDup("goalie")
	dup.Base = opts.base
	if err := dup.Apply(file, nil); err != nil {
		return fail(stderr, err)
	}
	found, err := dup.Run(dir, stdout, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	if found {
		return exitFailed
	}
	return exitClean
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "goalie: %v\n", err)
	return exitError
}
