package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/services/store-service/internal/app"
)

// EventPublisher publishes store events to NATS.
type EventPublisher struct {
	conn                    *nats.Conn
	itemsReservedSubj       string
	reservationFailedSubj   string
	orderConfirmedSubj      string
	reservationReleasedSubj string
}

// NewEventPublisher creates a new NATS EventPublisher.
func NewEventPublisher(
	conn *nats.Conn,
	itemsReservedSubj,
	reservationFailedSubj,
	orderConfirmedSubj,
	reservationReleasedSubj string,
) *EventPublisher {
	return &EventPublisher{
		conn:                    conn,
		itemsReservedSubj:       itemsReservedSubj,
		reservationFailedSubj:   reservationFailedSubj,
		orderConfirmedSubj:      orderConfirmedSubj,
		reservationReleasedSubj: reservationReleasedSubj,
	}
}

func (p *EventPublisher) PublishItemsReserved(_ context.Context, event app.ItemsReservedEvent) error {
	return p.publish(p.itemsReservedSubj, event)
}

func (p *EventPublisher) PublishReservationFailed(_ context.Context, event app.ReservationFailedEvent) error {
	return p.publish(p.reservationFailedSubj, event)
}

func (p *EventPublisher) PublishOrderConfirmed(_ context.Context, event app.OrderConfirmedEvent) error {
	return p.publish(p.orderConfirmedSubj, event)
}

func (p *EventPublisher) PublishReservationReleased(_ context.Context, event app.ReservationReleasedEvent) error {
	return p.publish(p.reservationReleasedSubj, event)
}

func (p *EventPublisher) publish(subject string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := p.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}
