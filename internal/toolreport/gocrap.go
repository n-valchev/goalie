package toolreport

import "fmt"

// CrapRow is one scored function. It has no limit, because a run that scores every function
// reports a limit of 0, which says nothing about the limits of the repo.
type CrapRow struct {
	File       string  `json:"file"`
	Line       int     `json:"line"`
	Function   string  `json:"function"`
	Score      float64 `json:"score"`
	Complexity int     `json:"complexity"`
	Coverage   float64 `json:"coverage"`
}

// ReadGocrap is strict about the shape, because any other JSON file, such as a godry report passed
// by mistake, would otherwise turn every function red with no error.
func ReadGocrap(dir, file string) ([]CrapRow, error) {
	var report struct {
		Findings []CrapRow `json:"findings"`
	}
	if err := readJSON(dir, file, &report); err != nil {
		return nil, err
	}
	if report.Findings == nil {
		return nil, fmt.Errorf("%s: not a gocrap report: no findings", file)
	}
	for i, row := range report.Findings {
		if row.File == "" || row.Line <= 0 {
			return nil, fmt.Errorf("%s: not a gocrap report: finding %d has no file or line", file, i)
		}
	}
	return report.Findings, nil
}
