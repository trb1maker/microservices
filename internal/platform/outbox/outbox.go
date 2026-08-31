package outbox

import (
	"context"
	"time"
	"uuid"
)

type Message struct {
	ID          int64
	AggregateID uuid.UUID
	EventType   string
	Subject     string
	Payload     []byte
	CreatedAt   time.Time
	Attempts    int
}

type Store interface {
	ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]Message, error)
	MarkPublished(ctx context.Context, ids []int64) error
	Reschedule(ctx context.Context, id int64, nextAttempt time.Time, lastError string) error
}

type Publisher interface {
	Publish(ctx context.Context, msg Message) error
}

func Backoff(attempts int, base, maxDelay time.Duration) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := base
	for i := 1; i < attempts; i++ {
		if delay > maxDelay/2 {
			return maxDelay
		}
		delay *= 2
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}
