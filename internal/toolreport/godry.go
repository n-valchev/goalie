package toolreport

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
	return readFindings(dir, file, "dup", func(pair DryPair) string {
		if pair.Left.File == "" || pair.Right.File == "" {
			return "left or right file"
		}
		return ""
	})
}
