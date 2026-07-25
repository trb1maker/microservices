package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/notification-service/app"
)

type Subjects struct {
	OrderFinalized   string
	OrderCancelled   string
	PaymentSucceeded string
	RefundSucceeded  string
}

type Consumer struct {
	conn     *nats.Conn
	subjects Subjects
	svc      *app.NotificationService
	subs     []*nats.Subscription
}

func NewConsumer(conn *nats.Conn, subjects Subjects, svc *app.NotificationService) *Consumer {
	return &Consumer{conn: conn, subjects: subjects, svc: svc}
}

func (c *Consumer) Start() error {
	handlers := []struct {
		subject string
		handler nats.MsgHandler
	}{
		{c.subjects.OrderFinalized, c.handleOrderFinalized},
		{c.subjects.OrderCancelled, c.handleOrderCancelled},
		{c.subjects.PaymentSucceeded, c.handlePaymentSucceeded},
		{c.subjects.RefundSucceeded, c.handleRefundSucceeded},
	}

	for _, item := range handlers {
		sub, err := c.conn.Subscribe(item.subject, item.handler)
		if err != nil {
			c.Close()
			return fmt.Errorf("subscribe %s: %w", item.subject, err)
		}
		c.subs = append(c.subs, sub)
	}
	return nil
}

func (c *Consumer) Close() {
	for _, sub := range c.subs {
		if sub != nil {
			_ = sub.Unsubscribe()
		}
	}
}

func (c *Consumer) handleOrderFinalized(msg *nats.Msg) {
	var event app.OrderFinalized
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal order finalized", slog.Any("error", err))
		return
	}
	if err := c.svc.HandleOrderFinalized(context.Background(), event); err != nil {
		slog.Error("handle order finalized", slog.Any("error", err))
	}
}

func (c *Consumer) handleOrderCancelled(msg *nats.Msg) {
	var event app.OrderCancelled
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal order cancelled", slog.Any("error", err))
		return
	}
	if err := c.svc.HandleOrderCancelled(context.Background(), event); err != nil {
		slog.Error("handle order cancelled", slog.Any("error", err))
	}
}

func (c *Consumer) handlePaymentSucceeded(msg *nats.Msg) {
	var event app.PaymentSucceeded
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal payment succeeded", slog.Any("error", err))
		return
	}
	if err := c.svc.HandlePaymentSucceeded(context.Background(), event); err != nil {
		slog.Error("handle payment succeeded", slog.Any("error", err))
	}
}

func (c *Consumer) handleRefundSucceeded(msg *nats.Msg) {
	var event app.RefundSucceeded
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal refund succeeded", slog.Any("error", err))
		return
	}
	if err := c.svc.HandleRefundSucceeded(context.Background(), event); err != nil {
		slog.Error("handle refund succeeded", slog.Any("error", err))
	}
}
