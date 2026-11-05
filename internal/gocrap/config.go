package gocrap

import (
	"errors"
	"fmt"
	"strings"

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
	cliflags.Fill(set, "base", &opts.base, tool.Base)
	cliflags.Fill(set, "limit", &opts.limits.Limit, tool.Limit)
	cliflags.Fill(set, "format", &opts.format, tool.Format)
	cliflags.Fill(set, "warn-only", &opts.warnOnly, tool.WarnOnly)
	if tool.CoverCommand != nil {
		opts.coverCommand = *tool.CoverCommand
	}
	if !set["shared-limit"] {
		opts.limits.SharedLimit = tool.SharedLimit
	}
	if !set["cover-profiles"] {
		opts.coverProfiles = tool.CoverProfiles
	}
	if err := opts.Apply(file, set); err != nil {
		return err
	}
	opts.limits.Shared = opts.Shared
	if err := opts.validate(); err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	if err := opts.limits.Check(); err != nil {
		return fmt.Errorf("%s: %w", config.Name, err)
	}
	return nil
}

func checkSection(tool config.Gocrap) error {
	// Empty would otherwise mean a scan of all code, as with the --base flag.
	if tool.Base != nil && *tool.Base == "" {
		return errors.New("base must name a revision")
	}
	if tool.CoverCommand != nil && strings.TrimSpace(*tool.CoverCommand) == "" {
		return errors.New("coverCommand must not be empty")
	}
	// The file can't say which one wins, so a flag does not excuse this either.
	if tool.CoverCommand != nil && len(tool.CoverProfiles) > 0 {
		return errors.New("set coverCommand or coverProfiles, not both")
	}
	return nil
}
