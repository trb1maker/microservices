package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/internal/notification-service/app"
)

type recordingSink struct {
	calls []call
}

type call struct {
	eventType, message, orderID, userID, transactionID string
	amount                                             int64
}

func (s *recordingSink) Notify(_ context.Context, eventType, message, orderID, userID, transactionID string, amount int64) {
	s.calls = append(s.calls, call{eventType, message, orderID, userID, transactionID, amount})
}

func TestNotificationService_HandleOrderFinalized(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	svc := app.NewNotificationService(sink)

	err := svc.HandleOrderFinalized(context.Background(), app.OrderFinalized{
		OrderID: "order-1",
		UserID:  "user-1",
	})
	require.NoError(t, err)
	require.Len(t, sink.calls, 1)
	assert.Equal(t, "ORDER_FINALIZED", sink.calls[0].eventType)
	assert.Contains(t, sink.calls[0].message, "order-1")
}

func TestNotificationService_HandlePaymentSucceeded(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	svc := app.NewNotificationService(sink)

	err := svc.HandlePaymentSucceeded(context.Background(), app.PaymentSucceeded{
		OrderID:       "order-1",
		UserID:        "user-1",
		Amount:        1000,
		TransactionID: "tx-1",
	})
	require.NoError(t, err)
	require.Len(t, sink.calls, 1)
	assert.Equal(t, "PAYMENT_SUCCEEDED", sink.calls[0].eventType)
	assert.Equal(t, int64(1000), sink.calls[0].amount)
}

func TestNotificationService_InvalidPayload(t *testing.T) {
	t.Parallel()
	svc := app.NewNotificationService(&recordingSink{})
	err := svc.HandleOrderCancelled(context.Background(), app.OrderCancelled{})
	assert.Error(t, err)
}
