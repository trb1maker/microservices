//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	natsadapter "github.com/trb1maker/microservices/internal/notification-service/adapters/nats"
	"github.com/trb1maker/microservices/internal/notification-service/app"
	"github.com/trb1maker/microservices/tests/internal/natstest"
)

const (
	orderFinalizedSubject   = "orders.finalized"
	orderCancelledSubject   = "orders.cancelled"
	paymentSucceededSubject = "payment.succeeded"
	refundSucceededSubject  = "payment.refund_succeeded"
)

type recordingSink struct {
	ch chan appCall
}

type appCall struct {
	eventType string
	orderID   string
}

func (s *recordingSink) Notify(_ context.Context, eventType, _, orderID, _, _ string, _ int64) {
	select {
	case s.ch <- appCall{eventType: eventType, orderID: orderID}:
	default:
	}
}

func TestIntegration_AllNotificationSubjects(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	consumerCtx := context.WithoutCancel(ctx)

	container, err := tcnats.Run(ctx, natstest.Image, natstest.ContainerOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	url, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	client := natstest.NewClient(t, url)
	t.Cleanup(client.Conn().Close)

	sink := &recordingSink{ch: make(chan appCall, 5)}
	svc := app.NewNotificationService(sink)
	consumer := natsadapter.NewConsumer(client, natsadapter.Subjects{
		OrderFinalized:   orderFinalizedSubject,
		OrderCancelled:   orderCancelledSubject,
		PaymentSucceeded: paymentSucceededSubject,
		RefundSucceeded:  refundSucceededSubject,
	}, svc)
	require.NoError(t, consumer.Start(consumerCtx))
	t.Cleanup(consumer.Close)

	publish := func(subject string, payload any) {
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		require.NoError(t, client.Publish(ctx, subject, data))
	}

	publish(orderFinalizedSubject, app.OrderFinalized{OrderID: "o1", UserID: "u1"})
	publish(orderCancelledSubject, app.OrderCancelled{OrderID: "o2", UserID: "u2"})
	publish(paymentSucceededSubject, app.PaymentSucceeded{OrderID: "o3", UserID: "u3", Amount: 100, TransactionID: "tx1"})
	publish(refundSucceededSubject, app.RefundSucceeded{OrderID: "o4", UserID: "u4", Amount: 50, TransactionID: "tx2"})
	publish(orderFinalizedSubject, app.OrderFinalized{
		OrderID:     "o5",
		UserID:      "u5",
		TotalAmount: 1500,
		Status:      "CONFIRMED",
		FinalizedAt: time.Now().UTC().Format(time.RFC3339),
	})

	received := make(map[string]struct{})
	for range 5 {
		select {
		case call := <-sink.ch:
			received[call.eventType] = struct{}{}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for notification")
		}
	}

	assert.Contains(t, received, "ORDER_FINALIZED")
	assert.Contains(t, received, "ORDER_CANCELLED")
	assert.Contains(t, received, "PAYMENT_SUCCEEDED")
	assert.Contains(t, received, "REFUND_SUCCEEDED")
}
