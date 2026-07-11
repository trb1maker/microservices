package domain

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

var (
	ErrUserIDRequired = errors.New("userID is required")
	ErrEmptyCart      = errors.New("cart is empty")
	ErrItemNotFound   = errors.New("item not found in cart")
)

type UserID uuid.UUID

type Cart struct {
	userID    UserID
	items     []OrderItem
	updatedAt time.Time
}

func NewCart(userID UserID, items ...OrderItem) (*Cart, error) {
	if userID == (UserID{}) {
		return nil, ErrUserIDRequired
	}

	if items == nil {
		items = []OrderItem{}
	}

	return &Cart{
		userID:    userID,
		items:     items,
		updatedAt: time.Now(),
	}, nil
}

func ReconstituteCart(userID UserID, updatedAt time.Time, items ...OrderItem) (*Cart, error) {
	if userID == (UserID{}) {
		return nil, ErrUserIDRequired
	}

	if items == nil {
		items = []OrderItem{}
	}

	return &Cart{
		userID:    userID,
		items:     items,
		updatedAt: updatedAt,
	}, nil
}

func (c *Cart) UserID() UserID {
	return c.userID
}

func (c *Cart) Items() []OrderItem {
	return slices.Clone(c.items)
}

func (c *Cart) UpdatedAt() time.Time {
	return c.updatedAt
}

func (c *Cart) TotalPrice() int64 {
	totalPrice := int64(0)
	for _, item := range c.items {
		totalPrice += item.totalPrice
	}
	return totalPrice
}

func (c *Cart) AddItem(item OrderItem) error {
	index := slices.IndexFunc(c.items, func(current OrderItem) bool {
		return current.productID == item.productID
	})
	if index != -1 {
		merged, err := c.items[index].Merge(item)
		if err != nil {
			return err
		}
		c.items[index] = merged
	} else {
		c.items = append(c.items, item)
	}

	c.updatedAt = time.Now()

	return nil
}

func (c *Cart) RemoveItem(productID ProductID) error {
	index := slices.IndexFunc(c.items, func(current OrderItem) bool {
		return current.productID == productID
	})
	if index == -1 {
		return ErrItemNotFound
	}

	c.items = slices.Delete(c.items, index, index+1)
	c.updatedAt = time.Now()

	return nil
}

func (c *Cart) Checkout(orderID OrderID, deliveryAddress string, now time.Time) (*Order, error) {
	if len(c.items) == 0 {
		return nil, ErrEmptyCart
	}

	return NewOrder(
		orderID,
		c.userID,
		OrderStatusPending,
		PaymentID{},
		deliveryAddress,
		now,
		now,
		c.items...,
	)
}

func (c *Cart) Clear() {
	c.items = []OrderItem{}
	c.updatedAt = time.Now()
}
