package nats

import (
	"context"
	"fmt"

	natslib "github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	"github.com/trb1maker/microservices/internal/store-service/app"
)

type EventPublisher struct {
	client                  *natsx.Client
	itemsReservedSubj       string
	reservationFailedSubj   string
	orderConfirmedSubj      string
	reservationReleasedSubj string
}

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
	return p.publish(ctx, p.itemsReservedSubj, event, event.OrderID)
}

func (p *EventPublisher) PublishReservationFailed(ctx context.Context, event app.ReservationFailedEvent) error {
	return p.publish(ctx, p.reservationFailedSubj, event, event.OrderID)
}

func (p *EventPublisher) PublishOrderConfirmed(ctx context.Context, event app.OrderConfirmedEvent) error {
	return p.publish(ctx, p.orderConfirmedSubj, event, event.OrderID)
}

func (p *EventPublisher) PublishReservationReleased(ctx context.Context, event app.ReservationReleasedEvent) error {
	return p.publish(ctx, p.reservationReleasedSubj, event, event.OrderID)
}

func (p *EventPublisher) publish(ctx context.Context, subject string, event any, orderID string) error {
	headers := natslib.Header{}
	if orderID != "" {
		headers.Set(natsx.HeaderOrderID, orderID)
	}
	if err := p.client.PublishJSON(ctx, subject, event, headers); err != nil {
		return fmt.Errorf("publish store event: %w", err)
	}
	return nil
}
