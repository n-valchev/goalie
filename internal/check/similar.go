package check

import (
	"cmp"
	"fmt"
	"go/build"
	"io"
	"path"
	"path/filepath"

	"github.com/n-valchev/goalie/internal/compare"
	"github.com/n-valchev/goalie/internal/extract"
)

// Match is a function of the repo that dup would pair with the function asked about.
type Match struct {
	Score    float64 `json:"score"`
	File     string  `json:"file"`
	Line     int     `json:"line"`
	Function string  `json:"function"`
}

// SimilarToCode reads code as a function of the package in folder, because the package decides
// whether a function of the same name is a platform variant, which dup doesn't report.
func (d Dup) SimilarToCode(dir, folder string, code string, stderr io.Writer) ([]Match, error) {
	// The line comment numbers the lines of code from 1, so that an error points into code.
	source := "package " + packageName(dir, folder) + "\n//line code:1\n" + code
	functions, err := d.Parser.ParseSource(path.Join(folder, "code"), []byte(source))
	if err != nil {
		return nil, fmt.Errorf("code doesn't parse as a Go function: %w", err)
	}
	if len(functions) != 1 {
		return nil, fmt.Errorf("code must be one Go function with a body, got %d", len(functions))
	}
	return d.similar(dir, functions[0], stderr)
}

func (d Dup) SimilarToFunction(dir, file string, line int, stderr io.Writer) ([]Match, error) {
	source, err := d.Parser.Source(dir, file)
	if err != nil {
		return nil, err
	}
	functions, err := d.Parser.ParseSource(file, source)
	if err != nil {
		return nil, err
	}
	for _, fn := range functions {
		if fn.StartLine <= line && line <= fn.EndLine {
			return d.similar(dir, fn, stderr)
		}
	}
	return nil, fmt.Errorf("%s:%d is in no function with a body", file, line)
}

// packageName asks go/build, so that a file that the build leaves out, such as a generator, can't
// name the package. With no Go file, no function of the folder can be a variant of code, so any
// name serves.
func packageName(dir, folder string) string {
	pkg, _ := build.ImportDir(filepath.Join(dir, filepath.FromSlash(folder)), 0)
	return cmp.Or(pkg.Name, "main")
}

func (d Dup) similar(dir string, target extract.Function, stderr io.Writer) ([]Match, error) {
	all, _, err := d.repoCandidates(dir, stderr)
	if err != nil {
		return nil, err
	}
	own := d.candidates([]extract.Function{target})
	// A function under the size limits has no match, because dup would never report it.
	if len(own) == 0 {
		return nil, nil
	}
	// A function of the repo is in all twice. Find drops that pair, as it drops the variants of one
	// function for two platforms, because the package, the name, and the receiver are the same.
	all = append(all, own[0])
	pairs, _ := compare.DropWeak(
		compare.Find(all, compare.Only([]int{len(all) - 1}), d.Threshold),
		compare.WeakLimits{Anchors: d.WeakAnchors, Literals: d.WeakLiterals},
	)
	matches := make([]Match, 0, len(pairs))
	for _, pair := range pairs {
		// The target is new, so Find puts it on the left.
		match := pair.Right
		matches = append(matches, Match{Score: pair.Score, File: match.File, Line: match.StartLine, Function: name(match.Function)})
	}
	return matches, nil
}
