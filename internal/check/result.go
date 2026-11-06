package check

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/n-valchev/goalie/internal/config"
)

type Result struct {
	Rule    string
	Mode    config.Mode // Empty means that the mode of the rule decides.
	Value   float64
	Limit   float64
	File    string
	Line    int
	Message string
	Details any // The JSON fields that only this rule has.
}

// MarshalJSON puts the shared fields beside the details in one object, because godeps reads the
// details at the top level of each finding.
func (r Result) MarshalJSON() ([]byte, error) {
	details, err := json.Marshal(r.Details)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if err := json.Unmarshal(details, &fields); err != nil {
		return nil, err
	}
	fields["rule"], fields["mode"], fields["value"], fields["limit"] = r.Rule, r.Mode, r.Value, r.Limit
	return json.Marshal(fields)
}

// limitRatio counts a limit of 0 as 1, because a division by 0 would give no order.
func (r Result) limitRatio() float64 {
	return r.Value / cmp.Or(r.Limit, 1)
}

func (r Result) rank() int {
	if r.Mode == config.ModeFail {
		return 0
	}
	return 1
}

func SortResults(results []Result) {
	slices.SortStableFunc(results, func(a, b Result) int {
		return cmp.Or(
			cmp.Compare(a.rank(), b.rank()),
			cmp.Compare(b.limitRatio(), a.limitRatio()),
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
		)
	})
}

func WriteResults(w io.Writer, format Format, results []Result) error {
	if format == FormatJSON {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		// Never null, so that a reader can always loop over the list.
		return encoder.Encode(struct {
			Findings []Result `json:"findings"`
		}{append([]Result{}, results...)})
	}
	for _, result := range results {
		_, err := fmt.Fprintf(w, "%s:%d: %s %s: %s\n", result.File, result.Line, result.Mode, result.Rule, result.Message)
		if err != nil {
			return err
		}
	}
	return nil
}

// noResults writes to stderr, because stdout holds only results when several rules share it.
func noResults(stderr io.Writer, tool, rule, message string) {
	fmt.Fprintf(stderr, "%s: %s: %s\n", tool, rule, message)
}
