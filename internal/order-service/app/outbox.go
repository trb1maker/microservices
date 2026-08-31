package app

import (
	"context"
	"time"
	"uuid"

	"github.com/trb1maker/microservices/internal/order-service/domain"
)

const OutboxEventOrderCreated = "orders.created"

// OrderCreatedSubject configures the NATS subject stored in outbox rows.
type OrderCreatedSubject string

// OutboxMessage is a row waiting to be published to JetStream.
type OutboxMessage struct {
	ID          int64
	AggregateID uuid.UUID
	EventType   string
	Subject     string
	Payload     []byte
	CreatedAt   time.Time
}

// CheckoutWriter persists checkout and enqueues the created event atomically (or publishes immediately).
type CheckoutWriter interface {
	PersistCheckout(ctx context.Context, order *domain.Order, event OrderCreated, subject string) error
}

// OutboxRelay publishes unpublished outbox rows.
type OutboxRelay interface {
	Run(ctx context.Context) error
}
