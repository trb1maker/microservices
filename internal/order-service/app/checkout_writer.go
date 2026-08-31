package app

import (
	"context"
	"fmt"
	"uuid"

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

func (w *immediateCheckoutWriter) PersistWithOutbox(
	ctx context.Context,
	order *domain.Order,
	messages []OutboxEnqueue,
) error {
	if order != nil {
		if err := w.orders.Save(ctx, order); err != nil {
			return fmt.Errorf("save order: %w", err)
		}
	}
	return w.publishOutbox(ctx, messages)
}

func (w *immediateCheckoutWriter) EnqueueOutbox(ctx context.Context, _ uuid.UUID, messages []OutboxEnqueue) error {
	return w.publishOutbox(ctx, messages)
}

func (w *immediateCheckoutWriter) publishOutbox(ctx context.Context, messages []OutboxEnqueue) error {
	for _, msg := range messages {
		if err := w.publishOne(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func (w *immediateCheckoutWriter) publishOne(ctx context.Context, msg OutboxEnqueue) error {
	switch msg.EventType {
	case OutboxEventConfirmOrder:
		event, ok := msg.Event.(ConfirmOrder)
		if !ok {
			return ErrInvalidConfirmOutboxPayload
		}
		if err := w.events.PublishConfirmOrder(ctx, event); err != nil {
			return fmt.Errorf("publish confirm order: %w", err)
		}
		return nil
	case OutboxEventOrderFinalized:
		event, ok := msg.Event.(OrderFinalized)
		if !ok {
			return ErrInvalidFinalizedOutboxPayload
		}
		if err := w.events.PublishOrderFinalized(ctx, event); err != nil {
			return fmt.Errorf("publish order finalized: %w", err)
		}
		return nil
	case OutboxEventOrderCancelled:
		event, ok := msg.Event.(OrderCancelled)
		if !ok {
			return ErrInvalidCancelledOutboxPayload
		}
		if err := w.events.PublishOrderCancelled(ctx, event); err != nil {
			return fmt.Errorf("publish order cancelled: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedOutboxEventType, msg.EventType)
	}
}
