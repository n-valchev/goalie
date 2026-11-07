package cart

import (
	_ "example.com/lib/sub"
	"example.com/shop/billing/invoice"
	_ "example.com/shop/lib/text"
)

// Store has a method.
type Store interface {
	Save(id string, n int) error
	invoice.Maker
}

type Size int

func Sum(items []int, limit int) int {
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

//goalie:ignore crap fixture
func Skipped() {}

func Stub()
