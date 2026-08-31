package mongodb

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"uuid"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/trb1maker/microservices/internal/store-service/app"
)

type OutboxPublisher struct {
	coll                    *mongo.Collection
	itemsReservedSubj       string
	reservationFailedSubj   string
	orderConfirmedSubj      string
	reservationReleasedSubj string
}

func NewOutboxPublisher(
	db *mongo.Database,
	itemsReservedSubj,
	reservationFailedSubj,
	orderConfirmedSubj,
	reservationReleasedSubj string,
) *OutboxPublisher {
	return &OutboxPublisher{
		coll:                    db.Collection("outbox"),
		itemsReservedSubj:       itemsReservedSubj,
		reservationFailedSubj:   reservationFailedSubj,
		orderConfirmedSubj:      orderConfirmedSubj,
		reservationReleasedSubj: reservationReleasedSubj,
	}
}

func (p *OutboxPublisher) PublishItemsReserved(ctx context.Context, event app.ItemsReservedEvent) error {
	return p.enqueue(ctx, event.OrderID, "store.items_reserved", p.itemsReservedSubj, event)
}

func (p *OutboxPublisher) PublishReservationFailed(ctx context.Context, event app.ReservationFailedEvent) error {
	return p.enqueue(ctx, event.OrderID, "store.reservation_failed", p.reservationFailedSubj, event)
}

func (p *OutboxPublisher) PublishOrderConfirmed(ctx context.Context, event app.OrderConfirmedEvent) error {
	return p.enqueue(ctx, event.OrderID, "store.order_confirmed", p.orderConfirmedSubj, event)
}

func (p *OutboxPublisher) PublishReservationReleased(ctx context.Context, event app.ReservationReleasedEvent) error {
	return p.enqueue(ctx, event.OrderID, "store.reservation_released", p.reservationReleasedSubj, event)
}

func (p *OutboxPublisher) enqueue(ctx context.Context, orderID, eventType, subject string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal store outbox: %w", err)
	}
	aggregateID, err := uuid.Parse(orderID)
	if err != nil {
		aggregateID = uuid.NewV7()
	}
	_, err = p.coll.InsertOne(ctx, map[string]any{
		"_id":          time.Now().UnixNano(),
		"aggregate_id": aggregateID.String(),
		"event_type":   eventType,
		"subject":      subject,
		"payload":      payload,
		"created_at":   time.Now().UTC(),
		"published_at": nil,
		"attempts":     0,
		"last_error":   "",
	})
	if err != nil {
		return fmt.Errorf("insert store outbox: %w", err)
	}
	return nil
}
