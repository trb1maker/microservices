package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trb1maker/microservices/services/analytics-service/internal/app"
)

const hoursPerDay = 24

type SummaryRepository struct {
	pool *pgxpool.Pool
}

func NewSummaryRepository(pool *pgxpool.Pool) *SummaryRepository {
	return &SummaryRepository{pool: pool}
}

func (r *SummaryRepository) Ping(ctx context.Context) error {
	if err := r.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (r *SummaryRepository) RecordOrder(ctx context.Context, orderID string, amount int64, finalizedAt time.Time) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM processed_orders WHERE order_id = $1)`, orderID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check processed order: %w", err)
	}
	if exists {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit tx: %w", err)
		}
		return true, nil
	}

	_, err = tx.Exec(ctx, `INSERT INTO processed_orders (order_id, processed_at) VALUES ($1, $2)`, orderID, finalizedAt)
	if err != nil {
		return false, fmt.Errorf("insert processed order: %w", err)
	}

	date := finalizedAt.UTC().Truncate(hoursPerDay * time.Hour)
	avgValue := float64(amount)
	_, err = tx.Exec(ctx, `
		INSERT INTO daily_summary (date, total_orders, total_revenue, avg_order_value)
		VALUES ($1, 1, $2, $3)
		ON CONFLICT (date) DO UPDATE SET
			total_orders = daily_summary.total_orders + 1,
			total_revenue = daily_summary.total_revenue + EXCLUDED.total_revenue,
			avg_order_value = (daily_summary.total_revenue + EXCLUDED.total_revenue)::double precision / (daily_summary.total_orders + 1)
	`, date, amount, avgValue)
	if err != nil {
		return false, fmt.Errorf("upsert daily summary: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tx: %w", err)
	}
	return false, nil
}

var _ app.SummaryRepository = (*SummaryRepository)(nil)
