package grpc_test

import (
	"testing"
	"time"
	"uuid"

	grpcadapter "github.com/trb1maker/microservices/internal/order-service/adapters/grpc"
	"github.com/trb1maker/microservices/internal/order-service/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusHub_NotifyOrderStatus(t *testing.T) {
	t.Parallel()

	orderID := uuid.NewV7()
	hub := grpcadapter.NewStatusHub()
	updates, cancel := hub.Subscribe(orderID.String())
	defer cancel()

	now := time.Now()
	item, err := domain.NewOrderItem(domain.ProductID(uuid.NewV7()), 1, 100)
	require.NoError(t, err)

	order, err := domain.NewOrder(
		domain.OrderID(orderID),
		domain.UserID(uuid.NewV7()),
		domain.OrderStatusReserved,
		domain.PaymentID{},
		"Moscow",
		now,
		now,
		*item,
	)
	require.NoError(t, err)

	hub.NotifyOrderStatus(order)

	select {
	case update := <-updates:
		assert.Equal(t, orderID.String(), update.GetOrderId())
		assert.Equal(t, "RESERVED", update.GetStatus())
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for status update")
	}
}
