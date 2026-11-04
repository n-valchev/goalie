// Package files selects the Go files that a scan reads.
package files

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Rules holds the user's choices for the file-level skip rules.
type Rules struct {
	IncludeTests bool
	SkipTags     []string
	SkipFiles    []string
}

// Find returns the Go files under paths, as sorted slash-separated paths relative to dir.
// Relative paths resolve against dir, and an empty paths list means dir itself.
func Find(dir string, paths []string, rules Rules) ([]string, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	tags := append([]string{"ignore"}, rules.SkipTags...)
	// A set, because overlapping paths must not make a function its own duplicate.
	found := map[string]bool{}
	for _, root := range paths {
		if !filepath.IsAbs(root) {
			root = filepath.Join(dir, root)
		}
		err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Relative to dir, because the folders that hold the repo itself must not count.
			rel, err := filepath.Rel(dir, file)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if entry.IsDir() {
				if inSkippedDir(rel) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(rel, ".go") || inSkippedDir(path.Dir(rel)) {
				return nil
			}
			if strings.HasSuffix(rel, "_test.go") && !rules.IncludeTests {
				return nil
			}
			if matchesAny(rules.SkipFiles, rel) {
				return nil
			}
			source, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			skipped, err := skippedByHeader(rel, source, tags)
			if err != nil {
				return err
			}
			if !skipped {
				found[rel] = true
			}
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

// matchesAny tests a pattern with no slash against the file name alone, so that one pattern covers
// a file that every package repeats.
func matchesAny(patterns []string, rel string) bool {
	for _, pattern := range patterns {
		target := rel
		if !strings.Contains(pattern, "/") {
			target = path.Base(rel)
		}
		// The command-line module has already rejected bad patterns.
		if matched, _ := path.Match(pattern, target); matched {
			return true
		}
	}
	return false
}

// inSkippedDir checks every folder of dir, because an explicit path can start below a skipped one.
func inSkippedDir(dir string) bool {
	for _, name := range strings.Split(dir, "/") {
		switch {
		case name == "." || name == "..":
		case name == "vendor" || name == "testdata" || name == "node_modules":
			return true
		case strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_"):
			return true
		}
	}
	return false
}
