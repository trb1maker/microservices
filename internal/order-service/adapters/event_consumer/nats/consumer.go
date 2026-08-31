package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/order-service/app"
	"github.com/trb1maker/microservices/internal/platform/natsx"
)

type Subjects struct {
	ItemsReserved     string
	ReservationFailed string
	OrderConfirmed    string
}

type Consumer struct {
	client   *natsx.Client
	subjects Subjects
	carts    *app.CartService
	orders   *app.OrderService
	subs     []*natsx.Subscription
}

func NewConsumer(
	client *natsx.Client,
	subjects Subjects,
	carts *app.CartService,
	orders *app.OrderService,
) *Consumer {
	return &Consumer{
		client:   client,
		subjects: subjects,
		carts:    carts,
		orders:   orders,
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	handlers := []struct {
		subject string
		durable string
		handler natsx.Handler
	}{
		{c.subjects.ItemsReserved, "order-store-items-reserved", c.handleItemsReserved},
		{c.subjects.ReservationFailed, "order-store-reservation-failed", c.handleReservationFailed},
		{c.subjects.OrderConfirmed, "order-store-order-confirmed", c.handleOrderConfirmed},
	}

	for _, item := range handlers {
		stream := natsx.StreamForSubject(item.subject)
		sub, err := c.client.ConsumeDurable(ctx, stream, item.durable, item.subject, item.handler, natsx.DurableConsumerConfig{})
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

func (c *Consumer) handleItemsReserved(ctx context.Context, msg *nats.Msg) error {
	var event app.ItemsReserved
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal items reserved", slog.Any("error", err))
		return nil
	}

	if err := c.carts.HandleItemsReserved(ctx, event); err != nil {
		return fmt.Errorf("handle items reserved: %w", err)
	}
	return nil
}

func (c *Consumer) handleReservationFailed(ctx context.Context, msg *nats.Msg) error {
	var event app.ReservationFailed
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal reservation failed", slog.Any("error", err))
		return nil
	}

	if err := c.carts.HandleReservationFailed(ctx, event); err != nil {
		return fmt.Errorf("handle cart reservation failed: %w", err)
	}
	if err := c.orders.HandleReservationFailed(ctx, event, msgReceivedAt(msg)); err != nil {
		return fmt.Errorf("handle order reservation failed: %w", err)
	}
	return nil
}

func (c *Consumer) handleOrderConfirmed(ctx context.Context, msg *nats.Msg) error {
	var event app.OrderConfirmed
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal order confirmed", slog.Any("error", err))
		return nil
	}

	if err := c.orders.HandleOrderConfirmed(ctx, event, msgReceivedAt(msg)); err != nil {
		return fmt.Errorf("handle order confirmed: %w", err)
	}
	return nil
}

func msgReceivedAt(msg *nats.Msg) time.Time {
	if msg == nil {
		return time.Now()
	}
	// Use current time; JetStream redelivery preserves payload only.
	return time.Now()
}
