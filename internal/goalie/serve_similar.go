package goalie

import (
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/mcp"
)

func (s served) similarTool() mcp.Tool {
	return s.tool(mcp.Tool{
		Name: "find_similar",
		Description: "Find the functions of the repo that are similar to one Go function, with the comparison and the limits " +
			"of the dup rule of goalie check. Call it before you write a new function, and reuse a match instead of a copy. " +
			"Pass code, the source of the function, or file and line of a function that exists. " +
			"Gives up to 10 matches, the highest score first, with more: the count left out.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"dir"},
			"properties": map[string]any{
				"dir":  map[string]any{"type": "string", "description": "The absolute path of the folder that you work in. code is read as if it were in the package of this folder."},
				"code": map[string]any{"type": "string", "description": "The source of one Go function. It need not be in a file yet."},
				"file": map[string]any{"type": "string", "description": "A Go file, relative to dir, that holds the function."},
				"line": map[string]any{"type": "integer", "description": "Any line of the function in file, from 1."},
			},
		},
	}, s.similar)
}

type similarInput struct {
	Dir  string `json:"dir"`
	Code string `json:"code"`
	File string `json:"file"`
	Line int    `json:"line"`
}

type similarAnswer struct {
	Matches  []check.Match `json:"matches"`
	More     int           `json:"more"`
	Unparsed []string      `json:"unparsed"`
}

func (s served) similar(arguments json.RawMessage, run check.Scope) (string, error) {
	var input similarInput
	if err := decodeInput(arguments, &input); err != nil {
		return "", err
	}
	runDir, err := s.runDir(input.Dir)
	if err != nil {
		return "", err
	}
	file, err := config.Read(runDir)
	if err != nil {
		return "", err
	}
	// all, because a match can be any function of the repo, changed or not.
	dup, err := buildDup(file, options{all: true}, run)
	if err != nil {
		return "", err
	}
	matches, err := s.matches(dup, input, runDir)
	if err != nil {
		return "", err
	}
	shown := append([]check.Match{}, matches[:min(maxResults, len(matches))]...)
	encoded, err := json.Marshal(similarAnswer{Matches: shown, More: len(matches) - len(shown), Unparsed: run.Parser.Unparsed()})
	return string(encoded), err
}

func (s served) matches(dup check.Dup, input similarInput, runDir string) ([]check.Match, error) {
	switch {
	case input.Code != "" && (input.File != "" || input.Line != 0):
		return nil, errors.New("give code, or file and line, not both")
	case input.Code != "":
		folder, err := pathIn(input.Dir, runDir, ".")
		if err != nil {
			return nil, err
		}
		return dup.SimilarToCode(runDir, filepath.ToSlash(folder[0]), input.Code, s.stderr)
	case input.File == "" || input.Line < 1:
		return nil, errors.New("needs code, or file and line: line is the number of a line in the function, from 1")
	}
	file, err := pathIn(input.Dir, runDir, input.File)
	if err != nil {
		return nil, err
	}
	return dup.SimilarToFunction(runDir, filepath.ToSlash(file[0]), input.Line, s.stderr)
}
