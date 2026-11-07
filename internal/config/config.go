// Package config reads .goalierc, the settings file that all goalie tools share.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const Name = ".goalierc"

// Settings holds what describes the repo at the top level, what belongs to one rule in Rules, and
// what belongs to godeps in its own section. goalie and godeps both decode every section, so a
// misspelled setting fails in either.
type Settings struct {
	Base      string   `json:"base"`
	Paths     []string `json:"paths"`
	SkipTags  []string `json:"skipTags"`
	SkipFiles []string `json:"skipFiles"`
	Shared    []string `json:"shared"`
	Rules     Rules    `json:"rules"`
	Godeps    Godeps   `json:"godeps"`
}

// Godeps holds pointers because a setting of 0 or false must replace a default, and only a setting
// that the file leaves out must not.
type Godeps struct {
	Format       *string  `json:"format"`
	ArchGoReport *string  `json:"archGoReport"`
	CrapReport   *string  `json:"crapReport"`
	DupReport    *string  `json:"dupReport"`
	EditorURL    *string  `json:"editorURL"`
	Title        *string  `json:"title"`
	Foreign      []string `json:"foreign"`
	Omit         []string `json:"omit"`
	Order        []string `json:"order"`
}

// Read looks only in dir, for the same reason as files.ReadIgnoreFile. A misspelled setting is an
// error, because ignoring it would silently run the check with weaker limits.
func Read(dir string) (Settings, error) {
	var result Settings
	content, err := os.ReadFile(filepath.Join(dir, Name))
	if errors.Is(err, fs.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err := checkRemovedSections(content); err != nil {
		return result, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		if errors.Is(err, io.EOF) {
			return result, fmt.Errorf("%s: must hold a JSON object", Name)
		}
		return result, locate(content, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return result, fmt.Errorf("%s: extra content after the JSON object", Name)
	}
	return result, nil
}

var removedSections = []struct{ section, rule string }{{"godry", "dup"}, {"gocrap", "crap"}}

// checkRemovedSections runs before the strict decode, whose unknown-field error would not say where
// the settings went. Content that doesn't decode here gets its error from the strict decode.
func checkRemovedSections(content []byte) error {
	var sections map[string]json.RawMessage
	if json.Unmarshal(content, &sections) != nil {
		return nil
	}
	for _, removed := range removedSections {
		if _, ok := sections[removed.section]; ok {
			return fmt.Errorf("%s: move %q settings to \"rules.%s\"", Name, removed.section, removed.rule)
		}
	}
	return nil
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
		return fmt.Errorf("%s: %w", Name, err)
	}
	line := 1 + bytes.Count(content[:min(offset, int64(len(content)))], []byte("\n"))
	return fmt.Errorf("%s:%d: %w", Name, line, err)
}
