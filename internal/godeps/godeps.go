// Package godeps is the command-line module and the only public seam of the godeps tool.
package godeps

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/n-valchev/goalie/internal/model"
)

const (
	exitClean = 0
	exitError = 2
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
	module, err := readModule(dir)
	if err != nil {
		return fail(stderr, err)
	}
	paths, err := opts.Find(dir, false)
	if err != nil {
		return fail(stderr, err)
	}
	opts.WarnUnmatched(stderr, "godeps", paths)
	source := model.Source{
		Dir:       dir,
		Files:     paths,
		Module:    module,
		Title:     cmp.Or(opts.title, module),
		Shared:    opts.Shared,
		Foreign:   opts.foreign,
		Omit:      opts.omit,
		Order:     opts.order,
		EditorURL: cmp.Or(opts.editorURL, defaultEditorURL),
	}
	result, err := model.Build(source)
	if err != nil {
		return fail(stderr, err)
	}
	if err := joinArchGo(dir, opts.archGoReport, &result, source, stderr); err != nil {
		return fail(stderr, err)
	}
	if err := joinDry(dir, opts.dryReport, &result, source, stderr); err != nil {
		return fail(stderr, err)
	}
	warnUnmatchedOrder(stderr, result)
	if err := writeOutput(stdout, result, opts.format); err != nil {
		return fail(stderr, err)
	}
	return exitClean
}

// warnUnmatchedOrder warns and does not fail, because paths can limit a run to some of the boxes.
func warnUnmatchedOrder(stderr io.Writer, result model.Model) {
	for _, name := range result.Order {
		isNamed := func(box model.Package) bool { return box.Parent == "" && box.Name == name }
		if !slices.ContainsFunc(result.Packages, isNamed) {
			fmt.Fprintf(stderr, "godeps: order name %q matches no top-level box\n", name)
		}
	}
}

func writeOutput(w io.Writer, result model.Model, format string) error {
	if format == formatHTML {
		return errors.New("--format html is not available yet; use --format json")
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// readModule reads the line by hand, because golang.org/x/mod would be goalie's first dependency.
func readModule(dir string) (string, error) {
	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(content), "\n") {
		line, _, _ = strings.Cut(line, "//")
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "module" && fields[1] != "(" {
			return strings.Trim(fields[1], "\"`"), nil
		}
	}
	return "", errors.New("go.mod has no module line")
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "godeps: %v\n", err)
	return exitError
}
