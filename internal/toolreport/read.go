// Package toolreport reads the JSON reports of other tools into small types that the model can join.
package toolreport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// A relative file starts from dir, where every tool runs.
func readJSON(dir, file string, report any) error {
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(content, report); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return nil
}

// readFindings is strict about the shape, because any JSON decodes into this struct. A report of
// the wrong tool would otherwise turn every function red or hide every duplicate, with no error.
func readFindings[T any](dir, file, tool, needs string, complete func(T) bool) ([]T, error) {
	var report struct {
		Findings []T `json:"findings"`
	}
	if err := readJSON(dir, file, &report); err != nil {
		return nil, err
	}
	if report.Findings == nil {
		return nil, fmt.Errorf("%s: not a %s report: no findings", file, tool)
	}
	for i, finding := range report.Findings {
		if !complete(finding) {
			return nil, fmt.Errorf("%s: not a %s report: finding %d has no %s", file, tool, i, needs)
		}
	}
	return report.Findings, nil
}
