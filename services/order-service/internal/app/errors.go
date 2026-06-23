package app

import "errors"

var (
	ErrOrderNotFound           = errors.New("order not found")
	ErrDeliveryAddressRequired = errors.New("delivery address is required")
	ErrInvalidUserID           = errors.New("invalid user id")
	ErrInvalidProductID        = errors.New("invalid product id")
	ErrInvalidOrderID          = errors.New("invalid order id")
)
