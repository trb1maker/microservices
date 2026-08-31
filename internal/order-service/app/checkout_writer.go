package app

import (
	"context"
	"fmt"

	"github.com/trb1maker/microservices/internal/order-service/domain"
)

type immediateCheckoutWriter struct {
	orders OrderRepository
	events OrderEventPublisher
}

func NewImmediateCheckoutWriter(orders OrderRepository, events OrderEventPublisher) CheckoutWriter {
	return &immediateCheckoutWriter{orders: orders, events: events}
}

func (w *immediateCheckoutWriter) PersistCheckout(
	ctx context.Context,
	order *domain.Order,
	event OrderCreated,
	_ string,
) error {
	if err := w.orders.Save(ctx, order); err != nil {
		return fmt.Errorf("save order: %w", err)
	}
	if err := w.events.PublishOrderCreated(ctx, event); err != nil {
		if delErr := w.orders.Delete(ctx, order.OrderID()); delErr != nil {
			return fmt.Errorf("publish order created: %w (rollback failed: %w)", err, delErr)
		}
		return fmt.Errorf("publish order created: %w", err)
	}
	return nil
}
