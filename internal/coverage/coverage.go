// Package coverage reads Go coverage profiles and answers how much of a function they cover.
package coverage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type span struct {
	startLine, startColumn, endLine, endColumn int
}

type block struct {
	statements int
	covered    bool
}

// Profile holds the code blocks of each file. A CRAP score needs only "covered or not", which is
// why profiles with different -covermode values can merge.
type Profile map[string]map[span]block

// Read merges a profile file into p. A block counts as covered if any profile covers it.
func (p Profile) Read(file string) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if !strings.HasPrefix(lines[0], "mode: ") {
		return fmt.Errorf("%s: not a coverage profile: the first line must be the mode", file)
	}
	for number, line := range lines {
		// A later mode line is valid, because a common way to merge profiles is to join the files.
		if strings.HasPrefix(line, "mode: ") {
			continue
		}
		if err := p.add(line); err != nil {
			return fmt.Errorf("%s:%d: %w", file, number+1, err)
		}
	}
	return nil
}

func (p Profile) add(line string) error {
	name, where, found := cutLast(line, ":")
	fields := strings.Fields(where)
	if !found || len(fields) != 3 {
		return fmt.Errorf("bad coverage line %q", line)
	}
	var position span
	_, err := fmt.Sscanf(fields[0], "%d.%d,%d.%d",
		&position.startLine, &position.startColumn, &position.endLine, &position.endColumn)
	if err != nil {
		return fmt.Errorf("bad coverage line %q", line)
	}
	statements, err := strconv.Atoi(fields[1])
	if err != nil {
		return fmt.Errorf("bad coverage line %q", line)
	}
	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return fmt.Errorf("bad coverage line %q", line)
	}
	if p[name] == nil {
		p[name] = map[span]block{}
	}
	p[name][position] = block{statements, p[name][position].covered || count > 0}
	return nil
}

func cutLast(text, separator string) (before, after string, found bool) {
	index := strings.LastIndex(text, separator)
	if index < 0 {
		return text, "", false
	}
	return text[:index], text[index+len(separator):], true
}

// Local returns p keyed by slash-separated paths relative to dir, like the paths of files.Find.
// Profiles name files by import path, so the module's go.mod gives the mapping. A file outside the
// module is an error, because it could only be left out silently or scored as untested.
func (p Profile) Local(dir string) (Profile, error) {
	root, module, err := findModule(dir)
	if err != nil {
		return nil, err
	}
	local := make(Profile, len(p))
	for name, blocks := range p {
		inModule, ok := strings.CutPrefix(name, module+"/")
		if !ok {
			return nil, fmt.Errorf("coverage for %s is outside module %s; the crap rule supports one module", name, module)
		}
		rel, err := filepath.Rel(dir, filepath.Join(root, filepath.FromSlash(inModule)))
		if err != nil {
			return nil, err
		}
		local[filepath.ToSlash(rel)] = blocks
	}
	return local, nil
}

func findModule(dir string) (root, module string, err error) {
	for root = dir; ; root = filepath.Dir(root) {
		content, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err == nil {
			module, err := modulePath(string(content))
			return root, module, err
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}
		if root == filepath.Dir(root) {
			return "", "", fmt.Errorf("no go.mod in %s or above it; the crap rule needs a Go module", dir)
		}
	}
}

// modulePath reads go.mod by hand, because the parser for it is outside the standard library.
func modulePath(content string) (string, error) {
	for line := range strings.Lines(content) {
		line, _, _ = strings.Cut(line, "//")
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			if unquoted, err := strconv.Unquote(fields[1]); err == nil {
				return unquoted, nil
			}
			return fields[1], nil
		}
	}
	return "", errors.New("go.mod has no module line")
}

// Of returns the fraction of statements covered on the lines from start to end of file. With no
// statements it returns 0, because code that no profile mentions must count as untested.
func (p Profile) Of(file string, start, end int) float64 {
	var total, covered int
	for position, found := range p[file] {
		if position.startLine < start || position.endLine > end {
			continue
		}
		total += found.statements
		if found.covered {
			covered += found.statements
		}
	}
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total)
}
