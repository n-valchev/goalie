package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/n-valchev/godry/internal/files"
)

const configFile = ".godryrc"

// config holds pointers because a setting of 0 or false must replace a default, and only a setting
// that the file leaves out must not.
type config struct {
	Paths        []string `json:"paths"`
	Base         *string  `json:"base"`
	Threshold    *float64 `json:"threshold"`
	MinLines     *int     `json:"minLines"`
	MinNodes     *int     `json:"minNodes"`
	WeakAnchors  *int     `json:"weakAnchors"`
	WeakLiterals *float64 `json:"weakLiterals"`
	Format       *format  `json:"format"`
	WarnOnly     *bool    `json:"warnOnly"`
	IncludeTests *bool    `json:"includeTests"`
	SkipTags     []string `json:"skipTags"`
	SkipFiles    []string `json:"skipFiles"`
}

// readConfig looks only in dir, for the same reason as files.ReadIgnoreFile. A misspelled setting
// is an error, because ignoring it would silently run the check with weaker limits.
func readConfig(dir string) (config, error) {
	var result config
	content, err := os.ReadFile(filepath.Join(dir, configFile))
	if errors.Is(err, fs.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		if errors.Is(err, io.EOF) {
			return result, fmt.Errorf("%s: must hold a JSON object", configFile)
		}
		return result, locate(content, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return result, fmt.Errorf("%s: extra content after the JSON object", configFile)
	}
	return result, nil
}

func locate(content []byte, err error) error {
	var offset int64
	var syntax *json.SyntaxError
	var wrongType *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		offset = syntax.Offset
	case errors.As(err, &wrongType):
		offset = wrongType.Offset
	default:
		return fmt.Errorf("%s: %w", configFile, err)
	}
	line := 1 + bytes.Count(content[:min(offset, int64(len(content)))], []byte("\n"))
	return fmt.Errorf("%s:%d: %w", configFile, line, err)
}

// applyConfig runs after the flags are parsed and validated, so that a failure here is the file's.
func applyConfig(dir string, opts *options) error {
	file, err := readConfig(dir)
	if err != nil {
		return err
	}
	// Empty would otherwise mean whole-repo mode, as with the --base flag.
	if file.Base != nil && *file.Base == "" {
		return fmt.Errorf("%s: base must name a revision", configFile)
	}
	fill(opts, "base", &opts.base, file.Base)
	fill(opts, "threshold", &opts.threshold, file.Threshold)
	fill(opts, "min-lines", &opts.minLines, file.MinLines)
	fill(opts, "min-nodes", &opts.minNodes, file.MinNodes)
	fill(opts, "weak-anchors", &opts.weakAnchors, file.WeakAnchors)
	fill(opts, "weak-literals", &opts.weakLiterals, file.WeakLiterals)
	fill(opts, "format", &opts.format, file.Format)
	fill(opts, "warn-only", &opts.warnOnly, file.WarnOnly)
	fill(opts, "include-tests", &opts.includeTests, file.IncludeTests)
	if !opts.fromFlags["skip-tags"] {
		opts.skipTags = file.SkipTags
	}
	if len(opts.paths) == 0 {
		opts.paths = file.Paths
	}
	for _, pattern := range file.SkipFiles {
		cleaned, err := files.CleanPattern(pattern)
		if err != nil {
			return fmt.Errorf("%s: %w", configFile, err)
		}
		opts.skipFiles = append(opts.skipFiles, cleaned)
	}
	if err := opts.validate(); err != nil {
		return fmt.Errorf("%s: %w", configFile, err)
	}
	return nil
}

func fill[T any](opts *options, flagName string, target, value *T) {
	if value != nil && !opts.fromFlags[flagName] {
		*target = *value
	}
}
