package gen

func Gen(items []int, limit int) int {
	total := 0
	for index, item := range items {
		if item > limit {
			total += item * index
		} else if item < 0 {
			total -= item
		} else {
			total += limit - item
		}
	}
	if total > 1000 {
		return 1000
	}
	return total
}
