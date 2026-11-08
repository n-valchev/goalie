package config

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/n-valchev/goalie/internal/files"
)

// Repo holds the options that describe the repo and so mean the same in every tool.
type Repo struct {
	Paths     []string
	SkipTags  []string
	SkipFiles []string
	Shared    []string
}

// AddFlags registers the flags of Repo on a tool's flag set.
func (r *Repo) AddFlags(flags *flag.FlagSet) {
	flags.Func("skip-tags", "", func(value string) error {
		r.SkipTags = strings.Split(value, ",")
		return nil
	})
	flags.Func("skip-files", "", func(value string) error {
		cleaned, err := files.CleanPatterns(strings.Split(value, ","))
		r.SkipFiles = append(r.SkipFiles, cleaned...)
		return err
	})
	flags.Func("shared", "", func(value string) error {
		cleaned, err := files.CleanPatterns(strings.Split(value, ","))
		r.Shared = append(r.Shared, cleaned...)
		return err
	})
}

// Apply fills r from the settings file. Paths and flags on the command line replace the file,
// apart from skip patterns, which add up.
func (r *Repo) Apply(file Settings, set map[string]bool) error {
	if !set["skip-tags"] {
		r.SkipTags = file.SkipTags
	}
	if len(r.Paths) == 0 {
		r.Paths = file.Paths
	}
	skipFiles, err := files.CleanPatterns(file.SkipFiles)
	if err != nil {
		return fmt.Errorf("%s: %w", Name, err)
	}
	r.SkipFiles = append(r.SkipFiles, skipFiles...)
	if !set["shared"] {
		if r.Shared, err = files.CleanPatterns(file.Shared); err != nil {
			return fmt.Errorf("%s: %w", Name, err)
		}
	}
	return nil
}

// Find returns the Go files that the options and the ignore file select.
func (r Repo) Find(dir string, includeTests bool) ([]string, error) {
	ignored, err := files.ReadIgnoreFile(dir)
	if err != nil {
		return nil, err
	}
	return files.Find(dir, r.Paths, files.Rules{
		IncludeTests: includeTests,
		SkipTags:     r.SkipTags,
		SkipFiles:    append(ignored, r.SkipFiles...),
	})
}

// WarnUnmatched warns and does not fail, because paths can limit a run to code with no shared
// folder. Without it, a mistyped pattern would quietly label shared code as a product.
func (r Repo) WarnUnmatched(stderr io.Writer, tool string, paths []string) {
	for _, warning := range r.UnmatchedWarnings(tool, paths) {
		fmt.Fprint(stderr, warning)
	}
}

func (r Repo) UnmatchedWarnings(tool string, paths []string) []string {
	var warnings []string
	for _, pattern := range files.Unmatched(r.Shared, paths) {
		warnings = append(warnings, fmt.Sprintf("%s: shared pattern %q matches no file; to match a folder, write folder/**\n", tool, pattern))
	}
	return warnings
}
