package goalie

import (
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/extract"
	"github.com/n-valchev/goalie/internal/mcp"
)

func (s served) riskTool() mcp.Tool {
	return s.tool(mcp.Tool{
		Name: "risk",
		Description: "Give the risk of one Go function, with no test run: its complexity, the range of its crap score, " +
			"the coverage that it needs to pass the crap limit, its duplicates, and the rules that it is over. " +
			"Call it before you change a function, to know how careful to be and which tests it needs. " +
			"Pass file and line, or symbol.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"dir"},
			"properties": map[string]any{
				"dir":  map[string]any{"type": "string", "description": "The absolute path of the folder that you work in."},
				"file": map[string]any{"type": "string", "description": "A Go file, relative to dir, that holds the function."},
				"line": map[string]any{"type": "integer", "description": "Any line of the function in file, from 1."},
				"symbol": map[string]any{"type": "string", "description": "The function as folder.Function or folder.Type.Method, " +
					"such as internal/store.Cache.Get. The folder is the one that the file of a check result starts with."},
			},
		},
	}, s.risk)
}

type riskInput struct {
	Dir    string `json:"dir"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
}

type location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type riskAnswer struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Function   string `json:"function"`
	Complexity int    `json:"complexity"`
	// Coverage is always null, because only a test run can tell, and the tool runs none.
	Coverage         *float64     `json:"coverage"`
	CrapRange        [2]float64   `json:"crapRange"`
	RequiredCoverage *float64     `json:"requiredCoverage"`
	CanPass          bool         `json:"canPass"`
	Limit            float64      `json:"limit"`
	Duplicates       shownMatches `json:"duplicates"`
	Rules            []ruleOver   `json:"rules"`
	// AlsoAt holds the other functions that symbol names, such as the variants for other platforms.
	AlsoAt   []location `json:"alsoAt,omitempty"`
	Unparsed []string   `json:"unparsed"`
}

type ruleOver struct {
	Rule  string  `json:"rule"`
	Value float64 `json:"value"`
	Limit float64 `json:"limit"`
}

func (s served) risk(arguments json.RawMessage, run check.Scope) (string, error) {
	var input riskInput
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
	found, err := functionsAskedAbout(input, runDir, run.Parser)
	if err != nil {
		return "", err
	}
	fn := found[0]
	// all, as in find_similar, because a duplicate can be any function of the repo.
	dup, err := buildDup(file, options{all: true}, run)
	if err != nil {
		return "", err
	}
	matches, err := dup.Similar(runDir, fn, s.stderr)
	if err != nil {
		return "", err
	}
	crapRule, err := buildCrap(file, options{partial: true}, run)
	if err != nil {
		return "", err
	}
	estimate := crapRule.Estimate(fn)
	answer := riskAnswer{
		File: fn.File, Line: fn.StartLine, Function: estimate.Function, Complexity: estimate.Complexity,
		CrapRange: [2]float64{crap.Score(estimate.Complexity, 1), crap.Score(estimate.Complexity, 0)},
		CanPass:   estimate.CanPass, Limit: estimate.Limit,
		Duplicates: showMatches(matches),
	}
	// null when no coverage can help, so that the agent can't read a number as a goal.
	if estimate.CanPass {
		answer.RequiredCoverage = &estimate.NeededCoverage
	}
	if answer.Rules, err = rulesOver(file, run, fn, matches); err != nil {
		return "", err
	}
	for _, other := range found[1:] {
		answer.AlsoAt = append(answer.AlsoAt, location{other.File, other.StartLine})
	}
	answer.Unparsed = run.Parser.Unparsed()
	encoded, err := json.Marshal(answer)
	return string(encoded), err
}

// rulesOver builds the rules as goalie check --all builds them, so that a rule that is off is left
// out here too. It leaves out the file rules and the type rules, because they are about the file or
// the type, and check reports them. crap is left out, because requiredCoverage already gives it.
func rulesOver(file config.Settings, run check.Scope, fn extract.Function, matches []check.Match) ([]ruleOver, error) {
	selected, err := selectRules(file, options{all: true, partial: true}, run)
	if err != nil {
		return nil, err
	}
	over := []ruleOver{}
	for _, entry := range selected {
		switch rule := entry.check.(type) {
		case check.Dup:
			// Similar gives the highest score first.
			if len(matches) > 0 && !extract.Excuses(fn.Decl, check.DupRule) {
				over = append(over, ruleOver{check.DupRule, matches[0].Score, rule.Threshold})
			}
		case *check.Function:
			if result, ok := rule.Over(fn); ok {
				over = append(over, ruleOver{result.Rule, result.Value, result.Limit})
			}
		}
	}
	return over, nil
}

// The first function is the one that the answer is about. The others go in alsoAt.
func functionsAskedAbout(input riskInput, runDir string, parser *extract.Parser) ([]extract.Function, error) {
	switch {
	case input.Symbol != "" && (input.File != "" || input.Line != 0):
		return nil, errors.New("give symbol, or file and line, not both")
	case input.Symbol != "":
		return check.FunctionsNamed(parser, runDir, input.Symbol)
	case input.File == "" || input.Line < 1:
		return nil, errors.New("needs symbol, or file and line: line is the number of a line in the function, from 1")
	}
	path, err := pathIn(input.Dir, runDir, input.File)
	if err != nil {
		return nil, err
	}
	fn, err := check.FunctionAt(parser, runDir, filepath.ToSlash(path[0]), input.Line)
	return []extract.Function{fn}, err
}
