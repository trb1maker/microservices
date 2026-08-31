package pgstore

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trb1maker/microservices/internal/platform/outbox"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]outbox.Message, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin claim tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const query = `
		SELECT id, aggregate_id, event_type, subject, payload, created_at, attempts
		FROM outbox
		WHERE published_at IS NULL
		  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	rows, err := tx.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	defer rows.Close()

	messages := make([]outbox.Message, 0, limit)
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var msg outbox.Message
		if err := rows.Scan(&msg.ID, &msg.AggregateID, &msg.EventType, &msg.Subject, &msg.Payload, &msg.CreatedAt, &msg.Attempts); err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		msg.Attempts++
		messages = append(messages, msg)
		ids = append(ids, msg.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	const leaseQ = `
		UPDATE outbox
		SET attempts = attempts + 1,
		    next_attempt_at = now() + $2
		WHERE id = ANY($1)`
	if _, err := tx.Exec(ctx, leaseQ, ids, lease); err != nil {
		return nil, fmt.Errorf("lease outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claim tx: %w", err)
	}
	return messages, nil
}

func (s *Store) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	const query = `UPDATE outbox SET published_at = now() WHERE id = ANY($1)`
	if _, err := s.pool.Exec(ctx, query, ids); err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

func (s *Store) Reschedule(ctx context.Context, id int64, nextAttempt time.Time, lastError string) error {
	const query = `UPDATE outbox SET next_attempt_at = $2, last_error = $3 WHERE id = $1`
	if _, err := s.pool.Exec(ctx, query, id, nextAttempt, lastError); err != nil {
		return fmt.Errorf("reschedule outbox: %w", err)
	}
	return nil
}

var _ outbox.Store = (*Store)(nil)
