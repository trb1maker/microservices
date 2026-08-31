package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	orderpostgres "github.com/trb1maker/microservices/internal/order-service/adapters/order_repository/postgres"
	"github.com/trb1maker/microservices/internal/order-service/app"
	"github.com/trb1maker/microservices/internal/order-service/domain"
)

// Writer persists checkout and outbox row in one transaction.
type Writer struct {
	pool      *pgxpool.Pool
	orderRepo *orderpostgres.OrderRepository
}

func NewWriter(pool *pgxpool.Pool) *Writer {
	return &Writer{
		pool:      pool,
		orderRepo: orderpostgres.NewOrderRepository(pool),
	}
}

func (w *Writer) PersistCheckout(
	ctx context.Context,
	order *domain.Order,
	event app.OrderCreated,
	subject string,
) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := w.orderRepo.SaveTx(ctx, tx, order); err != nil {
		return fmt.Errorf("save order: %w", err)
	}

	const insertOutbox = `
		INSERT INTO outbox (aggregate_id, event_type, subject, payload)
		VALUES ($1, $2, $3, $4)`

	if _, err := tx.Exec(ctx, insertOutbox,
		uuid.UUID(order.OrderID()),
		app.OutboxEventOrderCreated,
		subject,
		payload,
	); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit checkout tx: %w", err)
	}
	return nil
}

var _ app.CheckoutWriter = (*Writer)(nil)
