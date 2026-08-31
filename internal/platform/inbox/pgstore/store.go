package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/trb1maker/microservices/internal/platform/inbox"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Seen(ctx context.Context, consumer, eventID string) (bool, error) {
	var exists int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM inbox WHERE consumer = $1 AND event_id = $2`, consumer, eventID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("query inbox: %w", err)
	}
	return true, nil
}

func (s *Store) Mark(ctx context.Context, consumer, eventID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO inbox (consumer, event_id)
		VALUES ($1, $2)
		ON CONFLICT (consumer, event_id) DO NOTHING`, consumer, eventID)
	if err != nil {
		return fmt.Errorf("insert inbox: %w", err)
	}
	return nil
}

var _ inbox.Store = (*Store)(nil)
