// Package report describes a duplicate pair, in a result's message and in its JSON fields.
package report

import (
	"fmt"

	"github.com/n-valchev/goalie/internal/compare"
)

// Summary leaves out the file and line of the pair, because the report that uses it gives them.
func Summary(pair compare.Pair, limit float64) string {
	return fmt.Sprintf("duplicates %s:%d (score %.2f, limit %g, %s)",
		pair.Right.File, pair.Right.StartLine, pair.Score, limit, evidence(pair))
}

func evidence(pair compare.Pair) string {
	return fmt.Sprintf("literals %s, anchors %d%s", literalText(pair.Literals), pair.Anchors, caseText(pair.Case))
}

func literalText(score *float64) string {
	if score == nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", *score)
}

func caseText(found compare.Case) string {
	if found == "" {
		return ""
	}
	return ", case " + string(found)
}

type jsonFinding struct {
	Score    float64  `json:"score"`
	Literals *float64 `json:"literals"`
	Anchors  int      `json:"anchors"`
	Case     string   `json:"case,omitempty"`
	Left     jsonSide `json:"left"`
	Right    jsonSide `json:"right"`
}

type jsonSide struct {
	File      string `json:"file"`
	Function  string `json:"function"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Nodes     int    `json:"nodes"`
	New       bool   `json:"new"`
}

func Finding(pair compare.Pair) jsonFinding {
	return jsonFinding{pair.Score, pair.Literals, pair.Anchors, string(pair.Case), toJSON(pair.Left), toJSON(pair.Right)}
}

func toJSON(side compare.Side) jsonSide {
	return jsonSide{side.File, side.Name, side.StartLine, side.EndLine, side.Nodes, side.New}
}
