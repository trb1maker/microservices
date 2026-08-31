package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	orderpostgres "github.com/trb1maker/microservices/internal/order-service/adapters/order_repository/postgres"
	"github.com/trb1maker/microservices/internal/order-service/app"
	"github.com/trb1maker/microservices/internal/order-service/domain"
)

var errOrderRequired = errors.New("order is required")

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

func (w *Writer) PersistWithOutbox(ctx context.Context, order *domain.Order, messages []app.OutboxEnqueue) error {
	return w.persistOutbox(ctx, order, messages)
}

func (w *Writer) EnqueueOutbox(ctx context.Context, aggregateID uuid.UUID, messages []app.OutboxEnqueue) error {
	return w.insertOutboxMessages(ctx, nil, aggregateID, messages)
}

func (w *Writer) persistOutbox(ctx context.Context, order *domain.Order, messages []app.OutboxEnqueue) error {
	if order == nil {
		return errOrderRequired
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := w.orderRepo.SaveTx(ctx, tx, order); err != nil {
		return fmt.Errorf("save order: %w", err)
	}
	aggregateID := uuid.UUID(order.OrderID())

	if err := w.insertOutboxMessages(ctx, tx, aggregateID, messages); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit outbox tx: %w", err)
	}
	return nil
}

func (w *Writer) insertOutboxMessages(ctx context.Context, tx pgx.Tx, aggregateID uuid.UUID, messages []app.OutboxEnqueue) error {
	const insertOutbox = `
		INSERT INTO outbox (aggregate_id, event_type, subject, payload)
		VALUES ($1, $2, $3, $4)`
	for _, msg := range messages {
		payload, err := json.Marshal(msg.Event)
		if err != nil {
			return fmt.Errorf("marshal outbox payload: %w", err)
		}
		var execErr error
		if tx != nil {
			_, execErr = tx.Exec(ctx, insertOutbox, aggregateID, msg.EventType, msg.Subject, payload)
		} else {
			_, execErr = w.pool.Exec(ctx, insertOutbox, aggregateID, msg.EventType, msg.Subject, payload)
		}
		if execErr != nil {
			return fmt.Errorf("insert outbox: %w", execErr)
		}
	}
	return nil
}

var _ app.CheckoutWriter = (*Writer)(nil)
