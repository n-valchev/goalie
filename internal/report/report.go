// Package report writes findings.
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/compare"
)

func Text(w io.Writer, pairs []compare.Pair) error {
	if len(pairs) == 0 {
		_, err := fmt.Fprintln(w, "No duplicates found.")
		return err
	}
	for _, pair := range pairs {
		_, err := fmt.Fprintf(w, "%s:%d: duplicates %s:%d (score %.2f, %s)\n",
			pair.Left.File, pair.Left.StartLine, pair.Right.File, pair.Right.StartLine, pair.Score, evidence(pair))
		if err != nil {
			return err
		}
	}
	return nil
}

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

type jsonReport struct {
	Findings []jsonFinding `json:"findings"`
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

func JSON(w io.Writer, pairs []compare.Pair) error {
	findings := make([]jsonFinding, 0, len(pairs))
	for _, pair := range pairs {
		findings = append(findings, Finding(pair))
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonReport{findings})
}

func toJSON(side compare.Side) jsonSide {
	return jsonSide{side.File, side.Name, side.StartLine, side.EndLine, side.Nodes, side.New}
}
