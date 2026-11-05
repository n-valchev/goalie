package godry

import (
	"fmt"

	"github.com/n-valchev/goalie/internal/cliflags"
	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/files"
)

// applyConfig runs after the flags are parsed and validated, so that a failure here is the file's.
func applyConfig(dir string, opts *options) error {
	file, err := config.Read(dir)
	if err != nil {
		return err
	}
	tool, set := file.Godry, opts.fromFlags
	// Empty would otherwise mean whole-repo mode, as with the --base flag.
	if tool.Base != nil && *tool.Base == "" {
		return fmt.Errorf("%s: base must name a revision", config.Name)
	}
	cliflags.Fill(set, "base", &opts.base, tool.Base)
	cliflags.Fill(set, "threshold", &opts.threshold, tool.Threshold)
	cliflags.Fill(set, "min-lines", &opts.minLines, tool.MinLines)
	cliflags.Fill(set, "min-nodes", &opts.minNodes, tool.MinNodes)
	cliflags.Fill(set, "weak-anchors", &opts.weakAnchors, tool.WeakAnchors)
	cliflags.Fill(set, "weak-literals", &opts.weakLiterals, tool.WeakLiterals)
	cliflags.Fill(set, "format", (*string)(&opts.format), tool.Format)
	cliflags.Fill(set, "warn-only", &opts.warnOnly, tool.WarnOnly)
	cliflags.Fill(set, "include-tests", &opts.includeTests, tool.IncludeTests)
	if !set["skip-tags"] {
		opts.skipTags = file.SkipTags
	}
	if len(opts.paths) == 0 {
		opts.paths = file.Paths
	}
	skipFiles, err := files.CleanPatterns(file.SkipFiles)
	if err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	opts.skipFiles = append(opts.skipFiles, skipFiles...)
	if !set["shared"] {
		if opts.shared, err = files.CleanPatterns(file.Shared); err != nil {
			return fmt.Errorf("%s: %w", config.Name, err)
		}
	}
	if err := opts.validate(); err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	return nil
}
