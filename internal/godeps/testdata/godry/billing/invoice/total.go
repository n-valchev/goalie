package invoice

func Total(values []int, limit int) int {
	total := 0
	for _, value := range values {
		if value < 0 {
			continue
		}
		total += value
	}
	if total > limit {
		return limit
	}
	return total
}
