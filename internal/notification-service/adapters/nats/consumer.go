package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/trb1maker/microservices/internal/notification-service/app"
	"github.com/trb1maker/microservices/internal/platform/natsx"
)

var deliverNew = natsx.DurableConsumerConfig{DeliverPolicy: jetstream.DeliverNewPolicy}

type Subjects struct {
	OrderFinalized   string
	OrderCancelled   string
	PaymentSucceeded string
	RefundSucceeded  string
}

type Consumer struct {
	client   *natsx.Client
	subjects Subjects
	svc      *app.NotificationService
	subs     []*natsx.Subscription
}

func NewConsumer(client *natsx.Client, subjects Subjects, svc *app.NotificationService) *Consumer {
	return &Consumer{client: client, subjects: subjects, svc: svc}
}

func (c *Consumer) Start(ctx context.Context) error {
	handlers := []struct {
		subject string
		durable string
		handler natsx.Handler
	}{
		{c.subjects.OrderFinalized, "notification-orders-finalized", c.handleOrderFinalized},
		{c.subjects.OrderCancelled, "notification-orders-cancelled", c.handleOrderCancelled},
		{c.subjects.PaymentSucceeded, "notification-payment-succeeded", c.handlePaymentSucceeded},
		{c.subjects.RefundSucceeded, "notification-refund-succeeded", c.handleRefundSucceeded},
	}

	for _, item := range handlers {
		stream := natsx.StreamForSubject(item.subject)
		sub, err := c.client.ConsumeDurable(ctx, stream, item.durable, item.subject, item.handler, deliverNew)
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
			sub.Stop()
		}
	}
	c.subs = nil
}

func (c *Consumer) handleOrderFinalized(ctx context.Context, msg *nats.Msg) error {
	var event app.OrderFinalized
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal order finalized", slog.Any("error", err))
		return nil
	}
	if err := c.svc.HandleOrderFinalized(ctx, event); err != nil {
		return fmt.Errorf("handle order finalized: %w", err)
	}
	return nil
}

func (c *Consumer) handleOrderCancelled(ctx context.Context, msg *nats.Msg) error {
	var event app.OrderCancelled
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal order cancelled", slog.Any("error", err))
		return nil
	}
	if err := c.svc.HandleOrderCancelled(ctx, event); err != nil {
		return fmt.Errorf("handle order cancelled: %w", err)
	}
	return nil
}

func (c *Consumer) handlePaymentSucceeded(ctx context.Context, msg *nats.Msg) error {
	var event app.PaymentSucceeded
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal payment succeeded", slog.Any("error", err))
		return nil
	}
	if err := c.svc.HandlePaymentSucceeded(ctx, event); err != nil {
		return fmt.Errorf("handle payment succeeded: %w", err)
	}
	return nil
}

func (c *Consumer) handleRefundSucceeded(ctx context.Context, msg *nats.Msg) error {
	var event app.RefundSucceeded
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal refund succeeded", slog.Any("error", err))
		return nil
	}
	if err := c.svc.HandleRefundSucceeded(ctx, event); err != nil {
		return fmt.Errorf("handle refund succeeded: %w", err)
	}
	return nil
}
