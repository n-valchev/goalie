package gocrap

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/n-valchev/goalie/internal/coverage"
)

const coverDirVariable = "GOCRAP_COVER_DIR"

var globEscaper = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`)

// -coverpkg, because without it Go counts coverage only in the package that holds the test.
const defaultCoverCommand = `go test -coverpkg=./... -coverprofile="$` + coverDirVariable + `/coverage.out" ./...`

func gather(dir string, opts options, stderr io.Writer) (coverage.Profile, error) {
	profile := coverage.Profile{}
	if len(opts.coverProfiles) > 0 {
		for _, entry := range opts.coverProfiles {
			files, err := expand(dir, entry)
			if err != nil {
				return nil, err
			}
			// Merging is idempotent, so a file that two entries match needs no de-duplication.
			for _, file := range files {
				if err := profile.Read(file); err != nil {
					return nil, err
				}
			}
		}
		return profile.Local(dir)
	}
	// An empty folder of our own, so that a profile from an earlier run can never affect a score.
	coverDir, err := os.MkdirTemp("", "gocrap")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(coverDir)
	command := exec.Command("sh", "-c", opts.coverCommand)
	command.Dir = dir
	command.Env = append(os.Environ(), coverDirVariable+"="+coverDir)
	// Not stdout, which must stay the report for anything that reads it.
	command.Stdout, command.Stderr = stderr, stderr
	// A failure stops the run, because a suite that stopped early leaves low coverage and false scores.
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("coverage command failed: %w", err)
	}
	if err := readFolder(profile, coverDir); err != nil {
		return nil, err
	}
	return profile.Local(dir)
}

// expand keeps a plain path as it is, so that a missing file fails when it is read. A glob that
// matches nothing is an error, because a mistyped pattern would otherwise give false low coverage.
func expand(dir, entry string) ([]string, error) {
	absolute := filepath.IsAbs(entry)
	if !strings.ContainsAny(entry, `*?[\`) {
		if !absolute {
			entry = filepath.Join(dir, entry)
		}
		return []string{entry}, nil
	}
	pattern := entry
	if !absolute {
		// Only what the user typed is a pattern: a repo can live in a folder named `[work]`.
		pattern = filepath.Join(globEscaper.Replace(dir), entry)
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("cover profile pattern %q: %w", entry, err)
	}
	var files []string
	for _, match := range matches {
		if info, err := os.Stat(match); err == nil && info.Mode().IsRegular() {
			files = append(files, match)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("cover profile pattern %q matches no file", entry)
	}
	return files, nil
}

// readFolder skips subfolders, so that a command can keep raw GOCOVERDIR data there.
func readFolder(profile coverage.Profile, folder string) error {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return err
	}
	read := 0
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		if err := profile.Read(filepath.Join(folder, entry.Name())); err != nil {
			return err
		}
		read++
	}
	if read == 0 {
		return errors.New("the coverage command wrote no profile to " + coverDirVariable)
	}
	return nil
}
