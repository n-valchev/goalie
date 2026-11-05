package web

import "example.com/shop/domain"

type Page struct {
	domain.Order
	Title string
}

func Open() *Page { return nil }
