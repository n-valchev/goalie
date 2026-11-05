package toolreport

import "fmt"

// DryPair is one finding of godry.
type DryPair struct {
	Score float64 `json:"score"`
	Left  DrySide `json:"left"`
	Right DrySide `json:"right"`
}

// DrySide is one function of a pair. Function is the bare name, with no receiver.
type DrySide struct {
	File      string `json:"file"`
	Function  string `json:"function"`
	StartLine int    `json:"startLine"`
}

func ReadDry(dir, file string) ([]DryPair, error) {
	var report struct {
		Findings []DryPair `json:"findings"`
	}
	if err := readJSON(dir, file, &report); err != nil {
		return nil, err
	}
	// Any JSON decodes into this struct, and a gocrap report has findings too. Without this check,
	// a wrong path in the settings gives a map that silently shows no duplicates.
	if report.Findings == nil {
		return nil, fmt.Errorf("%s: not a godry report: no findings", file)
	}
	for _, pair := range report.Findings {
		if pair.Left.File == "" || pair.Right.File == "" {
			return nil, fmt.Errorf("%s: not a godry report: a finding has no left or right file", file)
		}
	}
	return report.Findings, nil
}
