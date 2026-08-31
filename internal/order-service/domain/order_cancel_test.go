package domain_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/trb1maker/microservices/internal/order-service/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrder_Cancel(t *testing.T) {
	t.Parallel()

	userID := domain.UserID(uuid.NewV7())
	item := mustOrderItem(t, domain.ProductID(uuid.NewV7()), 1, 100)
	now := time.Now()

	t.Run("cancels pending order", func(t *testing.T) {
		t.Parallel()

		order, err := domain.NewOrder(
			domain.OrderID(uuid.NewV7()),
			userID,
			domain.OrderStatusPending,
			domain.PaymentID{},
			"Moscow",
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
			domain.OrderID(uuid.NewV7()),
			userID,
			domain.OrderStatusConfirmed,
			domain.PaymentID(uuid.NewV7()),
			"Moscow",
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
