// Package files selects the Go files that a scan reads.
package files

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Rules holds the user's choices for the file-level skip rules.
type Rules struct {
	IncludeTests bool
	SkipTags     []string
}

// Find returns the Go files under paths, as sorted slash-separated paths relative to dir.
// Relative paths resolve against dir, and an empty paths list means dir itself.
func Find(dir string, paths []string, rules Rules) ([]string, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	// A set, because overlapping paths must not make a function its own duplicate.
	found := map[string]bool{}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		err := filepath.WalkDir(path, func(file string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(file, ".go") {
				return nil
			}
			rel, err := filepath.Rel(dir, file)
			if err != nil {
				return err
			}
			found[filepath.ToSlash(rel)] = true
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	result := make([]string, 0, len(found))
	for file := range found {
		result = append(result, file)
	}
	sort.Strings(result)
	return result, nil
}
