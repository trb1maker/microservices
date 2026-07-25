package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrder(t *testing.T) {
	t.Parallel()

	orderID := OrderID(uuid.New())
	userID := UserID(uuid.New())
	paymentID := PaymentID(uuid.New())
	item := mustOrderItem(t, ProductID(uuid.New()), 1, 100)
	now := time.Now()

	tests := []struct {
		name      string
		orderID   OrderID
		userID    UserID
		status    OrderStatus
		paymentID PaymentID
		items     []OrderItem
		wantErr   error
	}{
		{
			name:    "creates pending order with items",
			orderID: orderID,
			userID:  userID,
			status:  OrderStatusPending,
			items:   []OrderItem{item},
		},
		{
			name:      "creates paid order with paymentID",
			orderID:   orderID,
			userID:    userID,
			status:    OrderStatusPaid,
			paymentID: paymentID,
			items:     []OrderItem{item},
		},
		{
			name:    "requires orderID",
			orderID: OrderID{},
			userID:  userID,
			status:  OrderStatusPending,
			items:   []OrderItem{item},
			wantErr: ErrOrderIDRequired,
		},
		{
			name:    "requires userID",
			orderID: orderID,
			userID:  UserID{},
			status:  OrderStatusPending,
			items:   []OrderItem{item},
			wantErr: ErrUserIDRequired,
		},
		{
			name:    "requires items",
			orderID: orderID,
			userID:  userID,
			status:  OrderStatusPending,
			items:   nil,
			wantErr: ErrItemsRequired,
		},
		{
			name:      "rejects paymentID before payment",
			orderID:   orderID,
			userID:    userID,
			status:    OrderStatusPending,
			paymentID: paymentID,
			items:     []OrderItem{item},
			wantErr:   ErrPaymentIDNotAllowed,
		},
		{
			name:    "requires paymentID for paid order",
			orderID: orderID,
			userID:  userID,
			status:  OrderStatusPaid,
			items:   []OrderItem{item},
			wantErr: ErrPaymentIDRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			order, err := NewOrder(
				tt.orderID,
				tt.userID,
				tt.status,
				tt.paymentID,
				"Moscow",
				now,
				now,
				tt.items...,
			)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, order)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, order)
			assert.Equal(t, tt.orderID, order.OrderID())
			assert.Equal(t, tt.userID, order.UserID())
			assert.Equal(t, tt.status, order.Status())
			assert.Equal(t, tt.paymentID, order.PaymentID())
			assert.Equal(t, int64(100), order.TotalPrice())
			assert.Equal(t, tt.items, order.Items())
			assert.Equal(t, now, order.CreatedAt())
			assert.Equal(t, now, order.UpdatedAt())
		})
	}
}

func TestNewOrder_Items_returnsClone(t *testing.T) {
	t.Parallel()

	orderID := OrderID(uuid.New())
	userID := UserID(uuid.New())
	item := mustOrderItem(t, ProductID(uuid.New()), 1, 100)
	now := time.Now()

	order, err := NewOrder(orderID, userID, OrderStatusPending, PaymentID{}, "Moscow", now, now, item)
	require.NoError(t, err)

	items := order.Items()
	items[0] = mustOrderItem(t, ProductID(uuid.New()), 99, 1)

	assert.Equal(t, int64(1), order.Items()[0].Quantity())
}
