package goalie

import (
	"cmp"
	"encoding/json"

	"github.com/n-valchev/goalie/internal/check"
	"github.com/n-valchev/goalie/internal/config"
	"github.com/n-valchev/goalie/internal/mcp"
)

func (s served) rulesTool() mcp.Tool {
	return s.tool(mcp.Tool{
		Name: "rules",
		Description: "Give the limit, mode, and scope of each rule of goalie check, from the .goalierc nearest to dir " +
			"and the defaults. Call it at the start of a task, and write code within the limits.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"dir"},
			"properties": map[string]any{
				"dir": map[string]any{"type": "string", "description": "The absolute path of the folder that you work in."},
			},
		},
	}, s.rules)
}

type rulesInput struct {
	Dir string `json:"dir"`
}

// ruleSummary leaves out the settings of one rule, such as the dup thresholds, because the agent
// needs only the limit to write within it, and the rest would fill its context.
type ruleSummary struct {
	Rule  string       `json:"rule"`
	Limit float64      `json:"limit"`
	Mode  config.Mode  `json:"mode"`
	Scope config.Scope `json:"scope"`
}

type rulesAnswer struct {
	Rules []ruleSummary `json:"rules"`
}

func (s served) rules(arguments json.RawMessage, run check.Scope) (string, error) {
	var input rulesInput
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
	// Built as goalie check --all builds them, so that a limit that check rejects is an error here
	// too, and not a limit for the agent to write within.
	if _, err := selectRules(file, options{all: true}, run); err != nil {
		return "", err
	}
	var answer rulesAnswer
	for _, entry := range allRules {
		answer.Rules = append(answer.Rules, ruleSummary{
			Rule:  entry.name,
			Limit: entry.limit(file.Rules),
			Mode:  entry.mode(file.Rules),
			Scope: cmp.Or(entry.control(file.Rules).Scope, config.ScopeChanges),
		})
	}
	encoded, err := json.Marshal(answer)
	return string(encoded), err
}
