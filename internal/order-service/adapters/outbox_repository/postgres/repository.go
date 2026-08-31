package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trb1maker/microservices/internal/order-service/app"
)

// Repository reads and updates outbox rows.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ClaimUnpublished(ctx context.Context, limit int) ([]app.OutboxMessage, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin claim tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	messages, err := claimUnpublishedTx(ctx, tx, limit)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claim tx: %w", err)
	}
	return messages, nil
}

func (r *Repository) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	const query = `UPDATE outbox SET published_at = now() WHERE id = ANY($1)`
	if _, err := r.pool.Exec(ctx, query, ids); err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

// ProcessUnpublishedBatch claims rows under row lock, publishes them, then marks published atomically.
func (r *Repository) ProcessUnpublishedBatch(
	ctx context.Context,
	limit int,
	publish func(messages []app.OutboxMessage) ([]int64, error),
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin outbox batch tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	messages, err := claimUnpublishedTx(ctx, tx, limit)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	publishedIDs, err := publish(messages)
	if err != nil {
		return err
	}
	if err := markPublishedTx(ctx, tx, publishedIDs); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit outbox batch tx: %w", err)
	}
	return nil
}

func claimUnpublishedTx(ctx context.Context, tx pgx.Tx, limit int) ([]app.OutboxMessage, error) {
	const query = `
		SELECT id, aggregate_id, event_type, subject, payload, created_at
		FROM outbox
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	rows, err := tx.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	defer rows.Close()

	messages := make([]app.OutboxMessage, 0, limit)
	for rows.Next() {
		var msg app.OutboxMessage
		if err := rows.Scan(&msg.ID, &msg.AggregateID, &msg.EventType, &msg.Subject, &msg.Payload, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox: %w", err)
	}
	return messages, nil
}

func markPublishedTx(ctx context.Context, tx pgx.Tx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	const query = `UPDATE outbox SET published_at = now() WHERE id = ANY($1)`
	if _, err := tx.Exec(ctx, query, ids); err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

func (r *Repository) PendingCount(ctx context.Context) (int64, error) {
	const query = `SELECT COUNT(*) FROM outbox WHERE published_at IS NULL`
	var count int64
	if err := r.pool.QueryRow(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pending outbox: %w", err)
	}
	return count, nil
}
