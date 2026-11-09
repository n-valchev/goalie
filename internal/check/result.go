package check

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/n-valchev/goalie/internal/config"
)

// tool starts each line on stderr, so that a reader can tell goalie's lines from the test output
// that a coverage command writes there.
const tool = "goalie"

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

func (f Format) Validate() error {
	if f != FormatText && f != FormatJSON {
		return fmt.Errorf("invalid --format %q: must be text or json", string(f))
	}
	return nil
}

type Result struct {
	Rule    string
	Mode    config.Mode // Empty means that the mode of the rule decides.
	Value   float64
	Limit   float64
	File    string
	Line    int
	Message string
	Details any // The JSON fields that only this rule has.
	About   []Place
	Change  Change // Empty in goalie check, which has no base results to compare with.
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
	if r.Change != "" {
		fields["change"] = r.Change
	}
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

// changeRank puts what the change made after the old results that it touched, so that old results
// can't fill the places of a capped answer.
func (r Result) changeRank() int {
	if r.Change == ChangeUnchanged {
		return 1
	}
	return 0
}

func SortResults(results []Result) {
	slices.SortStableFunc(results, func(a, b Result) int {
		return cmp.Or(
			cmp.Compare(a.changeRank(), b.changeRank()),
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
func noResults(stderr io.Writer, rule, message string) {
	fmt.Fprintf(stderr, "%s: %s: %s\n", tool, rule, message)
}
