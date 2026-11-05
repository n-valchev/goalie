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

func ReadGocrap(dir, file string) ([]CrapRow, error) {
	return readFindings(dir, file, "gocrap", "file or line", func(row CrapRow) bool {
		return row.File != "" && row.Line > 0
	})
}
