package text

func Plain(s string) string {
	return s
}

func OrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func Bounded(s string) string {
	if s == "" {
		return "-"
	}
	if len(s) > 9 {
		return s[:9]
	}
	return s
}
