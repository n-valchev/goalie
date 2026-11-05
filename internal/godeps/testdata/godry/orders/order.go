package orders

type Order struct{}

func (o Order) Sum(values []int, limit int) int {
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

func (o Order) Empty() bool {
	return true
}
