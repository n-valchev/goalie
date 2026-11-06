package gocrap

import (
	"errors"
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
	tool, set := file.Gocrap, opts.fromFlags
	if err := checkSection(tool); err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	cliflags.Fill(set, "base", &opts.Base, tool.Base)
	cliflags.Fill(set, "limit", &opts.Limits.Limit, tool.Limit)
	cliflags.Fill(set, "format", (*string)(&opts.Format), tool.Format)
	cliflags.Fill(set, "warn-only", &opts.warnOnly, tool.WarnOnly)
	if tool.CoverCommand != nil {
		opts.CoverCommand = *tool.CoverCommand
	}
	if !set["shared-limit"] {
		opts.Limits.SharedLimit = tool.SharedLimit
	}
	if !set["cover-profiles"] {
		opts.CoverProfiles = tool.CoverProfiles
	}
	if err := opts.Apply(file, set); err != nil {
		return err
	}
	if err := opts.validate(); err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	if err := opts.Limits.Check(); err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	return nil
}

func checkSection(tool config.Gocrap) error {
	// Empty would otherwise mean a scan of all code, as with the --base flag.
	if tool.Base != nil && *tool.Base == "" {
		return errors.New("base must name a revision")
	}
	return tool.Cover.Check()
}
