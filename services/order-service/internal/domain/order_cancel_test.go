package domain_test

import (
	"order-service/internal/domain"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrder_Cancel(t *testing.T) {
	t.Parallel()

	userID := domain.UserID(uuid.New())
	item := mustOrderItem(t, domain.ProductID(uuid.New()), 1, 100)
	now := time.Now()

	t.Run("cancels pending order", func(t *testing.T) {
		t.Parallel()

		order, err := domain.NewOrder(
			domain.OrderID(uuid.New()),
			userID,
			domain.OrderStatusPending,
			domain.PaymentID{},
			now,
			now,
			item,
		)
		require.NoError(t, err)

		require.NoError(t, order.Cancel(now))
		assert.Equal(t, domain.OrderStatusCancelled, order.Status())
	})

	t.Run("forbids cancellation for confirmed order", func(t *testing.T) {
		t.Parallel()

		order, err := domain.NewOrder(
			domain.OrderID(uuid.New()),
			userID,
			domain.OrderStatusConfirmed,
			domain.PaymentID(uuid.New()),
			now,
			now,
			item,
		)
		require.NoError(t, err)

		err = order.Cancel(now)
		require.ErrorIs(t, err, domain.ErrOrderCancellationForbidden)
	})
}

func mustOrderItem(t *testing.T, productID domain.ProductID, quantity, unitPrice int64) domain.OrderItem {
	t.Helper()

	item, err := domain.NewOrderItem(productID, quantity, unitPrice)
	require.NoError(t, err)

	return *item
}
