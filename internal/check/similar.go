package check

import (
	"cmp"
	"fmt"
	"go/build"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

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
	return d.Similar(dir, functions[0], stderr)
}

func (d Dup) SimilarToFunction(dir, file string, line int, stderr io.Writer) ([]Match, error) {
	fn, err := FunctionAt(d.Parser, dir, file, line)
	if err != nil {
		return nil, err
	}
	return d.Similar(dir, fn, stderr)
}

// FunctionAt parses file again, and not from the parser's copy, so that a syntax error reaches the
// agent, who asked about this file.
func FunctionAt(parser *extract.Parser, dir, file string, line int) (extract.Function, error) {
	source, err := parser.Source(dir, file)
	if err != nil {
		return extract.Function{}, err
	}
	functions, err := parser.ParseSource(file, source)
	if err != nil {
		return extract.Function{}, err
	}
	for _, fn := range functions {
		if fn.StartLine <= line && line <= fn.EndLine {
			return fn, nil
		}
	}
	return extract.Function{}, fmt.Errorf("%s:%d is in no function with a body", file, line)
}

// FunctionsNamed finds the functions that symbol names, as folder.Function or folder.Type.Method,
// with folder relative to dir. A name can repeat in one package, with a variant for each platform, so
// it gives them all, in file order.
func FunctionsNamed(parser *extract.Parser, dir, symbol string) ([]extract.Function, error) {
	slash := strings.LastIndex(symbol, "/")
	folder, function, ok := strings.Cut(symbol[slash+1:], ".")
	folder = symbol[:slash+1] + folder
	var files []string
	// IsLocal keeps a symbol such as ../other.F out of the folders that the server may read.
	local := ok && filepath.IsLocal(filepath.FromSlash(folder))
	if entries, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(folder))); local && err == nil {
		for _, entry := range entries {
			if file := entry.Name(); !entry.IsDir() && strings.HasSuffix(file, ".go") && !strings.HasSuffix(file, "_test.go") {
				files = append(files, path.Join(folder, file))
			}
		}
	}
	// No rule, because a directive excuses a function from a rule, and not from the question.
	functions, err := parser.Parse(dir, files, "")
	if err != nil {
		return nil, err
	}
	functions = slices.DeleteFunc(functions, func(fn extract.Function) bool { return name(fn) != function })
	if len(functions) == 0 {
		return nil, fmt.Errorf("symbol %s names no function with a body: give folder.Function or "+
			"folder.Type.Method, with the folder relative to %s, or file and line for a function in that folder", symbol, dir)
	}
	return functions, nil
}

// packageName asks go/build, so that a file that the build leaves out, such as a generator, can't
// name the package. With no Go file, no function of the folder can be a variant of code, so any
// name serves.
func packageName(dir, folder string) string {
	pkg, _ := build.ImportDir(filepath.Join(dir, filepath.FromSlash(folder)), 0)
	return cmp.Or(pkg.Name, "main")
}

func (d Dup) Similar(dir string, target extract.Function, stderr io.Writer) ([]Match, error) {
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
