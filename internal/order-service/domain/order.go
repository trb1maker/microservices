package domain

import (
	"errors"
	"slices"
	"time"
	"uuid"
)

var (
	ErrOrderIDRequired            = errors.New("orderID is required")
	ErrItemsRequired              = errors.New("items are required")
	ErrTotalPriceInvalid          = errors.New("totalPrice must be greater than 0")
	ErrPaymentIDRequired          = errors.New("paymentID is required for paid order")
	ErrPaymentIDNotAllowed        = errors.New("paymentID is not allowed before payment")
	ErrOrderCancellationForbidden = errors.New("order cancellation is forbidden")
	ErrInvalidStatusTransition    = errors.New("invalid status transition")
)

type OrderID uuid.UUID

type PaymentID uuid.UUID

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"
	OrderStatusReserved  OrderStatus = "RESERVED"
	OrderStatusPaid      OrderStatus = "PAID"
	OrderStatusConfirmed OrderStatus = "CONFIRMED"
	OrderStatusCancelled OrderStatus = "CANCELLED"
)

type Order struct {
	orderID         OrderID
	userID          UserID
	items           []OrderItem
	totalPrice      int64
	status          OrderStatus
	paymentID       PaymentID
	deliveryAddress string
	createdAt       time.Time
	updatedAt       time.Time
	statusHistory   []StatusHistoryEntry
}

func NewOrder(
	orderID OrderID,
	userID UserID,
	status OrderStatus,
	paymentID PaymentID,
	deliveryAddress string,
	createdAt time.Time,
	updatedAt time.Time,
	items ...OrderItem,
) (*Order, error) {
	if orderID == (OrderID{}) {
		return nil, ErrOrderIDRequired
	}

	if userID == (UserID{}) {
		return nil, ErrUserIDRequired
	}

	if len(items) == 0 {
		return nil, ErrItemsRequired
	}

	totalPrice := int64(0)
	for _, item := range items {
		totalPrice += item.totalPrice
	}

	if totalPrice <= 0 {
		return nil, ErrTotalPriceInvalid
	}

	//nolint:exhaustive // Ложное срабатывание линтера
	switch status {
	case OrderStatusPaid, OrderStatusConfirmed:
		if paymentID == (PaymentID{}) {
			return nil, ErrPaymentIDRequired
		}
	case OrderStatusPending, OrderStatusReserved:
		if paymentID != (PaymentID{}) {
			return nil, ErrPaymentIDNotAllowed
		}
	}

	order := &Order{
		orderID:         orderID,
		userID:          userID,
		items:           items,
		totalPrice:      totalPrice,
		status:          status,
		paymentID:       paymentID,
		deliveryAddress: deliveryAddress,
		createdAt:       createdAt,
		updatedAt:       updatedAt,
		statusHistory: []StatusHistoryEntry{{
			Status:    status,
			CreatedAt: createdAt,
		}},
	}

	return order, nil
}

func ReconstituteOrder(
	orderID OrderID,
	userID UserID,
	status OrderStatus,
	paymentID PaymentID,
	deliveryAddress string,
	createdAt time.Time,
	updatedAt time.Time,
	statusHistory []StatusHistoryEntry,
	items ...OrderItem,
) (*Order, error) {
	order, err := NewOrder(orderID, userID, status, paymentID, deliveryAddress, createdAt, updatedAt, items...)
	if err != nil {
		return nil, err
	}
	if len(statusHistory) > 0 {
		order.statusHistory = slices.Clone(statusHistory)
	}
	return order, nil
}

func (o *Order) OrderID() OrderID {
	return o.orderID
}

func (o *Order) UserID() UserID {
	return o.userID
}

func (o *Order) Items() []OrderItem {
	return slices.Clone(o.items)
}

func (o *Order) TotalPrice() int64 {
	return o.totalPrice
}

func (o *Order) Status() OrderStatus {
	return o.status
}

func (o *Order) PaymentID() PaymentID {
	return o.paymentID
}

func (o *Order) DeliveryAddress() string {
	return o.deliveryAddress
}

func (o *Order) CreatedAt() time.Time {
	return o.createdAt
}

func (o *Order) UpdatedAt() time.Time {
	return o.updatedAt
}

func (o *Order) StatusHistory() []StatusHistoryEntry {
	return slices.Clone(o.statusHistory)
}

func (o *Order) MarkReserved(now time.Time) error {
	if o.status != OrderStatusPending {
		return ErrInvalidStatusTransition
	}
	o.status = OrderStatusReserved
	o.updatedAt = now
	o.appendHistory(OrderStatusReserved, "", now)
	return nil
}

func (o *Order) MarkPaid(paymentID PaymentID, now time.Time) error {
	if paymentID == (PaymentID{}) {
		return ErrPaymentIDRequired
	}
	if o.status != OrderStatusReserved {
		return ErrInvalidStatusTransition
	}
	o.status = OrderStatusPaid
	o.paymentID = paymentID
	o.updatedAt = now
	o.appendHistory(OrderStatusPaid, "", now)
	return nil
}

func (o *Order) MarkConfirmed(now time.Time) error {
	if o.status != OrderStatusPaid {
		return ErrInvalidStatusTransition
	}
	o.status = OrderStatusConfirmed
	o.updatedAt = now
	o.appendHistory(OrderStatusConfirmed, "", now)
	return nil
}

func (o *Order) Cancel(now time.Time) error {
	if o.status == OrderStatusConfirmed {
		return ErrOrderCancellationForbidden
	}
	if o.status == OrderStatusCancelled {
		return nil
	}
	o.status = OrderStatusCancelled
	o.updatedAt = now
	o.appendHistory(OrderStatusCancelled, "", now)
	return nil
}

func (o *Order) appendHistory(status OrderStatus, reason string, at time.Time) {
	o.statusHistory = append(o.statusHistory, StatusHistoryEntry{
		Status:    status,
		Reason:    reason,
		CreatedAt: at,
	})
}
