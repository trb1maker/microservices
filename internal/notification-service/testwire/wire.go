package testwire

import (
	"context"
	"fmt"
	"sync"

	natsadapter "github.com/trb1maker/microservices/internal/notification-service/adapters/nats"
	"github.com/trb1maker/microservices/internal/notification-service/app"
	"github.com/trb1maker/microservices/internal/platform/natsx"
)

type Subjects struct {
	OrderFinalized   string
	OrderCancelled   string
	PaymentSucceeded string
	RefundSucceeded  string
}

type RecordingSink struct {
	mu    sync.Mutex
	calls []NotificationCall
}

type NotificationCall struct {
	EventType     string
	OrderID       string
	UserID        string
	TransactionID string
	Amount        int64
}

func (s *RecordingSink) Notify(_ context.Context, eventType, _, orderID, userID, transactionID string, amount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, NotificationCall{
		EventType:     eventType,
		OrderID:       orderID,
		UserID:        userID,
		TransactionID: transactionID,
		Amount:        amount,
	})
}

func (s *RecordingSink) Calls() []NotificationCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]NotificationCall, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *RecordingSink) HasEvent(eventType string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, call := range s.calls {
		if call.EventType == eventType {
			return true
		}
	}
	return false
}

type Consumer struct {
	consumer *natsadapter.Consumer
	sink     *RecordingSink
}

func StartConsumer(ctx context.Context, client *natsx.Client, subjects Subjects) (*Consumer, error) {
	sink := &RecordingSink{}
	svc := app.NewNotificationService(sink)
	consumer := natsadapter.NewConsumer(client, natsadapter.Subjects{
		OrderFinalized:   subjects.OrderFinalized,
		OrderCancelled:   subjects.OrderCancelled,
		PaymentSucceeded: subjects.PaymentSucceeded,
		RefundSucceeded:  subjects.RefundSucceeded,
	}, svc)
	if err := consumer.Start(context.WithoutCancel(ctx)); err != nil {
		return nil, fmt.Errorf("start notification consumer: %w", err)
	}
	return &Consumer{consumer: consumer, sink: sink}, nil
}

func (c *Consumer) Sink() *RecordingSink {
	if c == nil {
		return nil
	}
	return c.sink
}

func (c *Consumer) Close() {
	if c == nil || c.consumer == nil {
		return
	}
	c.consumer.Close()
}
