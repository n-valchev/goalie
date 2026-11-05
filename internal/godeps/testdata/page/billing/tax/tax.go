package tax

import (
	_ "example.com/shop/billing/invoice"
	_ "example.com/shop/infra/db"
)

func Rate() int { return 1 }
