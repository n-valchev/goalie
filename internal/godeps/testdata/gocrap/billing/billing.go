package billing

type Invoice struct{ Total int }

func (i Invoice) Paid() bool {
	return i.Total == 0
}

func Sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

func Band(n int) string {
	if n < 0 {
		return "low"
	}
	if n > 9 {
		return "high"
	}
	return "mid"
}

func Size(n int) string {
	if n < 0 {
		return "none"
	}
	if n < 10 {
		return "small"
	}
	if n < 100 {
		return "medium"
	}
	return "large"
}

func Untested(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

//gocrap:ignore the fixture needs a function that the report leaves out
func Ignored() int {
	return 1
}
