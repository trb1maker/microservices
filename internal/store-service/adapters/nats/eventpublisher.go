package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trb1maker/microservices/internal/platform/natsx"
	"github.com/trb1maker/microservices/internal/store-service/app"
)

// EventPublisher publishes store events to JetStream.
type EventPublisher struct {
	client                  *natsx.Client
	itemsReservedSubj       string
	reservationFailedSubj   string
	orderConfirmedSubj      string
	reservationReleasedSubj string
}

// NewEventPublisher creates a new JetStream EventPublisher.
func NewEventPublisher(
	client *natsx.Client,
	itemsReservedSubj,
	reservationFailedSubj,
	orderConfirmedSubj,
	reservationReleasedSubj string,
) *EventPublisher {
	return &EventPublisher{
		client:                  client,
		itemsReservedSubj:       itemsReservedSubj,
		reservationFailedSubj:   reservationFailedSubj,
		orderConfirmedSubj:      orderConfirmedSubj,
		reservationReleasedSubj: reservationReleasedSubj,
	}
}

func (p *EventPublisher) PublishItemsReserved(ctx context.Context, event app.ItemsReservedEvent) error {
	return p.publish(ctx, p.itemsReservedSubj, event)
}

func (p *EventPublisher) PublishReservationFailed(ctx context.Context, event app.ReservationFailedEvent) error {
	return p.publish(ctx, p.reservationFailedSubj, event)
}

func (p *EventPublisher) PublishOrderConfirmed(ctx context.Context, event app.OrderConfirmedEvent) error {
	return p.publish(ctx, p.orderConfirmedSubj, event)
}

func (p *EventPublisher) PublishReservationReleased(ctx context.Context, event app.ReservationReleasedEvent) error {
	return p.publish(ctx, p.reservationReleasedSubj, event)
}

func (p *EventPublisher) publish(ctx context.Context, subject string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := p.client.Publish(ctx, subject, data); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}
