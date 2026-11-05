// Package report writes findings.
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/n-valchev/godry/internal/compare"
)

func Text(w io.Writer, pairs []compare.Pair) error {
	if len(pairs) == 0 {
		_, err := fmt.Fprintln(w, "No duplicates found.")
		return err
	}
	for _, pair := range pairs {
		_, err := fmt.Fprintf(w, "%s:%d: duplicates %s:%d (score %.2f, literals %s, anchors %d%s)\n",
			pair.Left.File, pair.Left.StartLine, pair.Right.File, pair.Right.StartLine,
			pair.Score, literalText(pair.Literals), pair.Anchors, caseText(pair.Case))
		if err != nil {
			return err
		}
	}
	return nil
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

func JSON(w io.Writer, pairs []compare.Pair) error {
	findings := make([]jsonFinding, 0, len(pairs))
	for _, pair := range pairs {
		findings = append(findings, jsonFinding{pair.Score, pair.Literals, pair.Anchors, string(pair.Case), toJSON(pair.Left), toJSON(pair.Right)})
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonReport{findings})
}

func toJSON(side compare.Side) jsonSide {
	return jsonSide{side.File, side.Name, side.StartLine, side.EndLine, side.Nodes, side.New}
}
