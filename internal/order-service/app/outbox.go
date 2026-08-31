package app

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/trb1maker/microservices/internal/order-service/domain"
)

var (
	ErrInvalidConfirmOutboxPayload   = errors.New("invalid confirm order outbox payload")
	ErrInvalidFinalizedOutboxPayload = errors.New("invalid order finalized outbox payload")
	ErrInvalidCancelledOutboxPayload = errors.New("invalid order cancelled outbox payload")
	ErrUnsupportedOutboxEventType    = errors.New("unsupported outbox event type")
)

const (
	OutboxEventOrderCreated   = "orders.created"
	OutboxEventConfirmOrder   = "orders.confirm"
	OutboxEventOrderFinalized = "orders.finalized"
	OutboxEventOrderCancelled = "orders.cancelled"
)

type OutboxEnqueue struct {
	EventType string
	Subject   string
	Event     any
}

// OrderCreatedSubject configures the NATS subject stored in outbox rows.
type OrderCreatedSubject string

type OrderEventSubjects struct {
	ConfirmOrder   string
	OrderFinalized string
	OrderCancelled string
}

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
	PersistWithOutbox(ctx context.Context, order *domain.Order, messages []OutboxEnqueue) error
	EnqueueOutbox(ctx context.Context, aggregateID uuid.UUID, messages []OutboxEnqueue) error
}

// OutboxRelay publishes unpublished outbox rows.
type OutboxRelay interface {
	Run(ctx context.Context) error
}
