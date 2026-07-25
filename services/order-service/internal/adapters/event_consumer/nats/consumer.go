package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/services/order-service/internal/app"
)

type Subjects struct {
	ItemsReserved     string
	ReservationFailed string
	OrderConfirmed    string
}

type Consumer struct {
	conn     *nats.Conn
	subjects Subjects
	carts    *app.CartService
	orders   *app.OrderService
	subs     []*nats.Subscription
}

func NewConsumer(
	conn *nats.Conn,
	subjects Subjects,
	carts *app.CartService,
	orders *app.OrderService,
) *Consumer {
	return &Consumer{
		conn:     conn,
		subjects: subjects,
		carts:    carts,
		orders:   orders,
	}
}

func (c *Consumer) Start() error {
	itemsReservedSub, err := c.conn.Subscribe(c.subjects.ItemsReserved, c.handleItemsReserved)
	if err != nil {
		return fmt.Errorf("subscribe items reserved: %w", err)
	}

	reservationFailedSub, err := c.conn.Subscribe(c.subjects.ReservationFailed, c.handleReservationFailed)
	if err != nil {
		_ = itemsReservedSub.Unsubscribe()
		return fmt.Errorf("subscribe reservation failed: %w", err)
	}

	orderConfirmedSub, err := c.conn.Subscribe(c.subjects.OrderConfirmed, c.handleOrderConfirmed)
	if err != nil {
		_ = itemsReservedSub.Unsubscribe()
		_ = reservationFailedSub.Unsubscribe()
		return fmt.Errorf("subscribe order confirmed: %w", err)
	}

	c.subs = []*nats.Subscription{itemsReservedSub, reservationFailedSub, orderConfirmedSub}
	return nil
}

func (c *Consumer) Close() {
	for _, sub := range c.subs {
		if sub != nil {
			_ = sub.Unsubscribe()
		}
	}
}

func (c *Consumer) handleItemsReserved(msg *nats.Msg) {
	var event app.ItemsReserved
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal items reserved", slog.Any("error", err))
		return
	}

	ctx := context.Background()
	if err := c.carts.HandleItemsReserved(ctx, event); err != nil {
		slog.Error("handle items reserved", slog.Any("error", err))
	}
}

func (c *Consumer) handleReservationFailed(msg *nats.Msg) {
	var event app.ReservationFailed
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal reservation failed", slog.Any("error", err))
		return
	}

	ctx := context.Background()
	if err := c.carts.HandleReservationFailed(ctx, event); err != nil {
		slog.Error("handle cart reservation failed", slog.Any("error", err))
	}
	if err := c.orders.HandleReservationFailed(ctx, event, time.Now()); err != nil {
		slog.Error("handle order reservation failed", slog.Any("error", err))
	}
}

func (c *Consumer) handleOrderConfirmed(msg *nats.Msg) {
	var event app.OrderConfirmed
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal order confirmed", slog.Any("error", err))
		return
	}

	ctx := context.Background()
	if err := c.orders.HandleOrderConfirmed(ctx, event, time.Now()); err != nil {
		slog.Error("handle order confirmed", slog.Any("error", err))
	}
}
