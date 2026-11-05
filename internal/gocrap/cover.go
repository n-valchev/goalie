package gocrap

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/n-valchev/goalie/internal/coverage"
)

const coverDirVariable = "GOCRAP_COVER_DIR"

// -coverpkg, because without it Go counts coverage only in the package that holds the test.
const defaultCoverCommand = `go test -coverpkg=./... -coverprofile="$` + coverDirVariable + `/coverage.out" ./...`

func gather(dir string, opts options, stderr io.Writer) (coverage.Profile, error) {
	profile := coverage.Profile{}
	if len(opts.coverProfiles) > 0 {
		for _, file := range opts.coverProfiles {
			if !filepath.IsAbs(file) {
				file = filepath.Join(dir, file)
			}
			if err := profile.Read(file); err != nil {
				return nil, err
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
