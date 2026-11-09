package check

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/n-valchev/goalie/internal/gitchange"
	"github.com/n-valchev/goalie/internal/rules"
)

type FileMeasure struct {
	Name         string
	DefaultLimit float64
	// Of returns false for a file with no value, which then reports nothing.
	Of       func(src []byte) (rules.FileMeasurement, bool)
	Describe func(file string, measured rules.FileMeasurement) string
}

var (
	LongFile = FileMeasure{
		Name: "long-file", DefaultLimit: 1000, Of: rules.FileLines,
		Describe: func(file string, measured rules.FileMeasurement) string {
			return fmt.Sprintf("%s has %.0f lines", file, measured.Value)
		},
	}
	HighMeanComplexity = FileMeasure{
		Name: "high-mean-complexity", DefaultLimit: 7, Of: rules.MeanComplexity,
		Describe: func(file string, measured rules.FileMeasurement) string {
			functions := fmt.Sprintf("%d functions", measured.Functions)
			if measured.Functions == 1 {
				functions = "1 function"
			}
			return fmt.Sprintf("%s has mean complexity %.1f over %s", file, measured.Value, functions)
		},
	}
)

type File struct {
	Limited
	Measure FileMeasure
}

func NewFile(scope Scope, measure FileMeasure) *File {
	return &File{Limited: Limited{Scope: scope, Name: measure.Name, Limit: measure.DefaultLimit}, Measure: measure}
}

type fileMeasured struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Functions int    `json:"functions,omitempty"`
}

// Results reports a file in scope changes only when the change takes it over the limit, so that an
// old long file doesn't report on every edit.
func (f File) Results(dir string, stderr io.Writer) ([]Result, error) {
	// An unchanged file has the value that it had at the base, so it can't cross the limit.
	paths, _, err := f.changedFiles(dir, stderr)
	if err != nil {
		return nil, err
	}
	var bases map[string]gitchange.Base
	if f.Base != "" {
		if bases, err = f.Changes.Base(dir, f.Base, paths); err != nil {
			// The rule must never fail the check for want of a base value.
			f.Warnings.warn(stderr, fmt.Sprintf("%s: file rules report nothing: can't read the files at --base: %v\n", tool, err))
			return nil, nil
		}
	}
	var results []Result
	for _, file := range paths {
		src, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
		if err != nil {
			return nil, err
		}
		measured, ok := f.Measure.Of(src)
		if !ok || measured.Value <= f.Limit || bases != nil && !f.crossed(bases, file) {
			continue
		}
		// Line 1, because the whole file is the problem.
		results = append(results, Result{
			Rule: f.Measure.Name, Value: measured.Value, Limit: f.Limit, File: file, Line: 1,
			Message: fmt.Sprintf("%s (limit %g)", f.Measure.Describe(file, measured), f.Limit),
			Details: fileMeasured{File: file, Line: 1, Functions: measured.Functions},
		})
	}
	return results, nil
}

// crossed is false for a file with no base value, because then no one can tell whether the change
// crossed the limit.
func (f File) crossed(bases map[string]gitchange.Base, file string) bool {
	base, ok := bases[file]
	if !ok {
		return false
	}
	if base.New {
		return true
	}
	measured, ok := f.Measure.Of(base.Content)
	return ok && measured.Value <= f.Limit
}
