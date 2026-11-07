package toolreport

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

// crapFinding takes the score from CrapRow, because only a pointer shows that a score is missing.
type crapFinding struct {
	CrapRow
	Score *float64 `json:"score"`
}

func ReadCrap(dir, file string) ([]CrapRow, error) {
	findings, err := readFindings(dir, file, "crap", func(found crapFinding) string {
		switch {
		case found.File == "" || found.Line <= 0:
			return "file or line"
		// A partial check scores nothing, and a score of 0 would grade every function green.
		case found.Score == nil:
			return "score: run goalie check crap without --partial"
		}
		return ""
	})
	if err != nil {
		return nil, err
	}
	rows := make([]CrapRow, len(findings))
	for i, found := range findings {
		rows[i] = found.CrapRow
		rows[i].Score = *found.Score
	}
	return rows, nil
}
