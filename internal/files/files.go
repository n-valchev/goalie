// Package files selects the Go files that a scan reads.
package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const ignoreFile = ".goalieignore"

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
			if MatchesAny(rules.SkipFiles, rel) {
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

// CleanPattern rejects the gitignore forms that the pattern rules don't support. Accepted, each
// would match nothing, so the user would believe a file was skipped.
func CleanPattern(pattern string) (string, error) {
	if strings.HasPrefix(pattern, "!") {
		return "", fmt.Errorf("unsupported pattern %q: ! is not supported", pattern)
	}
	if strings.HasSuffix(pattern, "/") {
		return "", fmt.Errorf("unsupported pattern %q: to skip a folder, write %s**", pattern, pattern)
	}
	// Each part alone, because matching splits at slashes, which would break a [a/b] class.
	for _, part := range strings.Split(pattern, "/") {
		if _, err := path.Match(part, ""); err != nil {
			return "", fmt.Errorf("bad pattern %q", pattern)
		}
		if part != "**" && strings.Contains(part, "**") {
			return "", fmt.Errorf("unsupported pattern %q: ** must be a whole path part", pattern)
		}
	}
	// Scanned paths never start with ./, so that spelling would otherwise match nothing.
	return path.Clean(pattern), nil
}

// CleanPatterns applies CleanPattern to each pattern.
func CleanPatterns(patterns []string) ([]string, error) {
	cleaned := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		result, err := CleanPattern(pattern)
		if err != nil {
			return nil, err
		}
		cleaned = append(cleaned, result)
	}
	return cleaned, nil
}

// Unmatched returns the patterns that match none of the given files.
func Unmatched(patterns, paths []string) []string {
	var unmatched []string
	for _, pattern := range patterns {
		if !slices.ContainsFunc(paths, func(rel string) bool { return MatchesAny([]string{pattern}, rel) }) {
			unmatched = append(unmatched, pattern)
		}
	}
	return unmatched
}

// ReadIgnoreFile returns the patterns in dir's .goalieignore. It looks in no other folder, because
// whole-repo mode has no repo root to stop at, and a search upward could find an unrelated file.
func ReadIgnoreFile(dir string) ([]string, error) {
	content, err := os.ReadFile(filepath.Join(dir, ignoreFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var patterns []string
	for number, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pattern, err := CleanPattern(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", ignoreFile, number+1, err)
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

// MatchesAny tests a pattern with no slash against the file name alone, so that one pattern covers
// a file that every package repeats.
func MatchesAny(patterns []string, rel string) bool {
	for _, pattern := range patterns {
		target := rel
		if !strings.Contains(pattern, "/") && pattern != "**" {
			target = path.Base(rel)
		}
		if matchParts(strings.Split(pattern, "/"), strings.Split(target, "/")) {
			return true
		}
	}
	return false
}

func matchParts(pattern, parts []string) bool {
	if len(pattern) == 0 {
		return len(parts) == 0
	}
	if pattern[0] == "**" {
		for skipped := 0; skipped <= len(parts); skipped++ {
			if matchParts(pattern[1:], parts[skipped:]) {
				return true
			}
		}
		return false
	}
	if len(parts) == 0 {
		return false
	}
	// CleanPattern has already rejected bad patterns.
	matched, _ := path.Match(pattern[0], parts[0])
	return matched && matchParts(pattern[1:], parts[1:])
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
