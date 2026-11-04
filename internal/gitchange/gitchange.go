// Package gitchange finds the changed line ranges of each file. It is the only package that runs git.
package gitchange

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Range is the lines from Start to End. A deletion is the empty range between two lines, so its
// End is Start-1.
type Range struct {
	Start, End int
}

// Changes holds the changed ranges of each file, keyed by slash-separated path.
type Changes map[string][]Range

// Touches reports whether a change falls inside the lines from start to end of file.
func (c Changes) Touches(file string, start, end int) bool {
	for _, r := range c[file] {
		if r.Start <= end && r.End >= start {
			return true
		}
	}
	return false
}

// Since returns what changed in the working tree since the merge base of rev and HEAD. Its paths
// are relative to dir, which may be any directory inside the repo.
func Since(dir, rev string) (Changes, error) {
	prefix, err := git(dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, fmt.Errorf("--base needs a Git repository: %w", err)
	}
	base, err := mergeBase(dir, rev)
	if err != nil {
		return nil, err
	}
	patch, err := git(dir, append(diffArgs, base)...)
	if err != nil {
		return nil, err
	}
	fromRoot, err := parsePatch(patch)
	if err != nil {
		return nil, err
	}
	// Without the :/ pathspec, ls-files lists only the files below dir.
	untracked, err := git(dir, "ls-files", "--others", "--exclude-standard", "--full-name", "-z", ":/")
	if err != nil {
		return nil, err
	}
	for _, file := range strings.Split(strings.TrimSuffix(untracked, "\x00"), "\x00") {
		fromRoot[file] = append(fromRoot[file], Range{1, math.MaxInt})
	}

	changes := Changes{}
	for file, ranges := range fromRoot {
		rel, err := filepath.Rel(strings.TrimSuffix(prefix, "\n"), file)
		if err != nil {
			return nil, err
		}
		changes[filepath.ToSlash(rel)] = ranges
	}
	return changes, nil
}

// diffArgs states every setting that could change the hunks or their headers, because user config
// and .gitattributes would otherwise decide them.
var diffArgs = []string{
	"diff",
	"--unified=0",
	"--inter-hunk-context=0",
	"--no-renames",
	"--no-relative",
	"--no-ext-diff",
	"--no-textconv",
	"--no-color",
	"--text",
	"--dst-prefix=b/",
	"--diff-algorithm=myers",
	"--indent-heuristic",
	"--submodule=short",
}

func mergeBase(dir, rev string) (string, error) {
	// The hash from this check is what reaches merge-base and diff, so that a rev such as
	// --output=file can't act as an option and write a file.
	commit, err := git(dir, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("unknown revision %q", rev)
	}
	base, err := git(dir, "merge-base", strings.TrimSpace(commit), "HEAD")
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", fmt.Errorf("no merge base between %q and HEAD: in a shallow clone, fetch more history", rev)
	}
	return strings.TrimSpace(base), err
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

func parsePatch(patch string) (Changes, error) {
	changes := Changes{}
	file := ""
	inHeader := false
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHeader = true
		// An added source line that starts with "++ " also starts with "+++ " here, so the header
		// state is what tells them apart.
		case inHeader && strings.HasPrefix(line, "+++ "):
			name, err := patchName(strings.TrimPrefix(line, "+++ "))
			if err != nil {
				return nil, err
			}
			file = name
		case strings.HasPrefix(line, "@@ "):
			inHeader = false
			r, err := parseHunk(line)
			if err != nil {
				return nil, err
			}
			if file != "" {
				changes[file] = append(changes[file], r)
			}
		}
	}
	return changes, nil
}

// patchName returns the path in a "+++" header, or "" for a deleted file.
func patchName(field string) (string, error) {
	// git pads a name that holds a space with one tab. A real tab in a name is always quoted.
	field = strings.TrimSuffix(field, "\t")
	if field == "/dev/null" {
		return "", nil
	}
	if strings.HasPrefix(field, `"`) {
		// core.quotePath=true leaves only ASCII and escapes that strconv reads byte for byte, so
		// a name that isn't valid UTF-8 survives too.
		unquoted, err := strconv.Unquote(field)
		if err != nil {
			return "", fmt.Errorf("unexpected git diff name %q: %w", field, err)
		}
		field = unquoted
	}
	return strings.TrimPrefix(field, "b/"), nil
}

func parseHunk(line string) (Range, error) {
	match := hunkHeader.FindStringSubmatch(line)
	if match == nil {
		return Range{}, fmt.Errorf("unexpected git diff line %q", line)
	}
	start, err := strconv.Atoi(match[1])
	if err != nil {
		return Range{}, err
	}
	count := 1
	if match[2] != "" {
		if count, err = strconv.Atoi(match[2]); err != nil {
			return Range{}, err
		}
	}
	if count == 0 {
		// For a pure deletion, git names the line before the gap, not the first line after it.
		return Range{start + 1, start}, nil
	}
	return Range{start, start + count - 1}, nil
}

// environment drops what points git at another repo, which a Git hook sets, and what overrides
// diffArgs. git reads GIT_DIFF_OPTS after --unified.
func environment(env []string) []string {
	return slices.DeleteFunc(env, func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_DIFF_OPTS":
			return true
		}
		return false
	})
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "core.quotePath=true"}, args...)...)
	cmd.Dir = dir
	cmd.Env = environment(cmd.Environ())
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, bytes.TrimSpace(exit.Stderr))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}
