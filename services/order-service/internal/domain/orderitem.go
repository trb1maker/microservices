package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type ProductID uuid.UUID

type OrderItem struct {
	productID  ProductID
	quantity   int64
	unitPrice  int64
	totalPrice int64
}

var (
	ErrNotValidOrderItem = errors.New("not valid order item")
	ErrProductIDMismatch = errors.New("productID mismatch")
	ErrUnitPriceMismatch = errors.New("unitPrice mismatch")
)

func NewOrderItem(productID ProductID, quantity int64, unitPrice int64) (*OrderItem, error) {
	if productID == (ProductID{}) {
		return nil, fmt.Errorf("%w: productID is required", ErrNotValidOrderItem)
	}

	if quantity <= 0 {
		return nil, fmt.Errorf("%w: quantity must be greater than 0", ErrNotValidOrderItem)
	}

	if unitPrice <= 0 {
		return nil, fmt.Errorf("%w: unitPrice must be greater than 0", ErrNotValidOrderItem)
	}

	return &OrderItem{
		productID:  productID,
		quantity:   quantity,
		unitPrice:  unitPrice,
		totalPrice: quantity * unitPrice,
	}, nil
}

func (i OrderItem) ProductID() ProductID {
	return i.productID
}

func (i OrderItem) Quantity() int64 {
	return i.quantity
}

func (i OrderItem) UnitPrice() int64 {
	return i.unitPrice
}

func (i OrderItem) TotalPrice() int64 {
	return i.totalPrice
}

func (i OrderItem) Merge(o OrderItem) (OrderItem, error) {
	if i.productID != o.productID {
		return OrderItem{}, ErrProductIDMismatch
	}

	if i.unitPrice != o.unitPrice {
		return OrderItem{}, ErrUnitPriceMismatch
	}

	quantity := i.quantity + o.quantity

	return OrderItem{
		productID:  i.productID,
		quantity:   quantity,
		unitPrice:  i.unitPrice,
		totalPrice: quantity * i.unitPrice,
	}, nil
}
