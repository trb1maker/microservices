package domain

import "errors"

var (
	ErrProductNotFound    = errors.New("product not found")
	ErrStockNotFound      = errors.New("stock not found for product")
	ErrInsufficientStock  = errors.New("insufficient stock available")
	ErrInvalidQuantity    = errors.New("quantity must be positive")
	ErrInvalidReservation = errors.New("invalid reservation state")
)
