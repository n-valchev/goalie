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
