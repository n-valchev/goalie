package domain

import _ "example.com/shop/infra/db"

type Order struct {
	ID string
}

func (o Order) Total() int { return 1 }
