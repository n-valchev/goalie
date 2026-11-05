package godry

import (
	"fmt"

	"github.com/n-valchev/goalie/internal/cliflags"
	"github.com/n-valchev/goalie/internal/config"
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
	if err := opts.Apply(file, set); err != nil {
		return err
	}
	if err := opts.validate(); err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	return nil
}
