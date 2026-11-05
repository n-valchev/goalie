package godeps

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
	tool, set := file.Godeps, opts.fromFlags
	cliflags.Fill(set, "format", &opts.format, tool.Format)
	cliflags.Fill(set, "arch-go-report", &opts.archGoReport, tool.ArchGoReport)
	cliflags.Fill(set, "crap-report", &opts.crapReport, tool.CrapReport)
	cliflags.Fill(set, "dry-report", &opts.dryReport, tool.DryReport)
	cliflags.Fill(set, "editor-url", &opts.editorURL, tool.EditorURL)
	cliflags.Fill(set, "title", &opts.title, tool.Title)
	opts.foreign, opts.order = tool.Foreign, tool.Order
	if opts.omit, err = files.CleanPatterns(tool.Omit); err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	if err := opts.Apply(file, set); err != nil {
		return err
	}
	if err := opts.validate(); err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	return nil
}
