package domain

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

var (
	ErrUserIDRequired       = errors.New("userID is required")
	ErrEmptyCart            = errors.New("cart is empty")
	ErrItemNotFound         = errors.New("item not found in cart")
	ErrCartNotFullyReserved = errors.New("cart is not fully reserved")
)

type Cart struct {
	userID         UserID
	pendingOrderID OrderID
	items          []OrderItem
	updatedAt      time.Time
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

func ReconstituteCart(userID UserID, pendingOrderID OrderID, updatedAt time.Time, items ...OrderItem) (*Cart, error) {
	if userID == (UserID{}) {
		return nil, ErrUserIDRequired
	}

	if items == nil {
		items = []OrderItem{}
	}

	return &Cart{
		userID:         userID,
		pendingOrderID: pendingOrderID,
		items:          items,
		updatedAt:      updatedAt,
	}, nil
}

func (c *Cart) UserID() UserID {
	return c.userID
}

func (c *Cart) PendingOrderID() OrderID {
	return c.pendingOrderID
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

func (c *Cart) EnsurePendingOrderID() OrderID {
	if c.pendingOrderID == (OrderID{}) {
		c.pendingOrderID = OrderID(uuid.New())
		c.updatedAt = time.Now()
	}
	return c.pendingOrderID
}

func (c *Cart) AllItemsReserved() bool {
	if len(c.items) == 0 {
		return false
	}
	for _, item := range c.items {
		if item.reservationStatus != ReservationStatusReserved {
			return false
		}
	}
	return true
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
		merged.reservationStatus = ReservationStatusPending
		c.items[index] = merged
	} else {
		item.reservationStatus = ReservationStatusPending
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

	if len(c.items) == 0 {
		c.pendingOrderID = OrderID{}
	}

	return nil
}

func (c *Cart) MarkItemReserved(productID ProductID) error {
	index := c.findItemIndex(productID)
	if index == -1 {
		return ErrItemNotFound
	}
	c.items[index].MarkReserved()
	c.updatedAt = time.Now()
	return nil
}

func (c *Cart) MarkItemFailed(productID ProductID) error {
	index := c.findItemIndex(productID)
	if index == -1 {
		return ErrItemNotFound
	}
	c.items[index].MarkFailed()
	c.updatedAt = time.Now()
	return nil
}

func (c *Cart) Checkout(deliveryAddress string, now time.Time) (*Order, error) {
	if len(c.items) == 0 {
		return nil, ErrEmptyCart
	}
	if !c.AllItemsReserved() {
		return nil, ErrCartNotFullyReserved
	}
	if c.pendingOrderID == (OrderID{}) {
		return nil, ErrCartNotFullyReserved
	}

	order, err := NewOrder(
		c.pendingOrderID,
		c.userID,
		OrderStatusReserved,
		PaymentID{},
		deliveryAddress,
		now,
		now,
		c.items...,
	)
	if err != nil {
		return nil, err
	}

	return order, nil
}

func (c *Cart) Clear() {
	c.items = []OrderItem{}
	c.pendingOrderID = OrderID{}
	c.updatedAt = time.Now()
}

func (c *Cart) findItemIndex(productID ProductID) int {
	return slices.IndexFunc(c.items, func(current OrderItem) bool {
		return current.productID == productID
	})
}
