package goalie

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/mcp"
)

// slowCall has no setting, because the log is for the maintainer of goalie: repeated lines are the
// sign that goalie needs an index.
var slowCall = 5 * time.Second

// toolCall reads the code through run, so that the log can count the files that it read.
type toolCall func(arguments json.RawMessage, run check.Scope) (string, error)

// A new run for each call, so that no answer reads a file as it was in an earlier call.
func (s served) tool(tool mcp.Tool, call toolCall) mcp.Tool {
	tool.Call = func(arguments json.RawMessage) (string, error) {
		run := newRun()
		run.Parser.SkipUnparsed()
		start := time.Now()
		text, err := call(arguments, run)
		// A slow answer still goes to the agent, because it is better than none.
		if took := time.Since(start); took > slowCall {
			s.logSlow(arguments, fmt.Sprintf("%s %s took %.1fs, read %d files\n",
				start.Format(time.RFC3339), tool.Name, took.Seconds(), run.Parser.FilesRead()))
		}
		return text, err
	}
	return tool
}

// logSlow sends a failure to stderr and not to the agent, because the answer is ready, and the log
// is for the maintainer.
func (s served) logSlow(arguments json.RawMessage, line string) {
	var input struct {
		Dir string `json:"dir"`
	}
	// An input that doesn't decode has no dir, and runDir says so.
	_ = json.Unmarshal(arguments, &input)
	runDir, err := s.runDir(input.Dir)
	if err == nil {
		err = appendSlow(filepath.Join(runDir, ".goalie"), line)
	}
	if err != nil {
		fail(s.stderr, fmt.Errorf("can't log the slow call: %w", err))
	}
}

// appendSlow writes the .gitignore first, so that the log never shows as a change to commit.
func appendSlow(folder, line string) error {
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	ignore := filepath.Join(folder, ".gitignore")
	if _, err := os.Stat(ignore); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
			return err
		}
	}
	log, err := os.OpenFile(filepath.Join(folder, "slow.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = log.WriteString(line)
	return errors.Join(err, log.Close())
}
