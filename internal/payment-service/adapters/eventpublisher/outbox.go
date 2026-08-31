package eventpublisher

import (
	"context"
	"encoding/json"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trb1maker/microservices/internal/payment-service/app"
)

type OutboxPublisher struct {
	pool                 *pgxpool.Pool
	paymentSucceededSubj string
	paymentFailedSubj    string
	refundSucceededSubj  string
	refundFailedSubj     string
}

func NewOutboxPublisher(
	pool *pgxpool.Pool,
	paymentSucceededSubj,
	paymentFailedSubj,
	refundSucceededSubj,
	refundFailedSubj string,
) *OutboxPublisher {
	return &OutboxPublisher{
		pool:                 pool,
		paymentSucceededSubj: paymentSucceededSubj,
		paymentFailedSubj:    paymentFailedSubj,
		refundSucceededSubj:  refundSucceededSubj,
		refundFailedSubj:     refundFailedSubj,
	}
}

func (p *OutboxPublisher) PublishPaymentSucceeded(ctx context.Context, event app.PaymentSucceededEvent) error {
	return p.enqueue(ctx, event.OrderID, "payment.succeeded", p.paymentSucceededSubj, event)
}

func (p *OutboxPublisher) PublishPaymentFailed(ctx context.Context, event app.PaymentFailedEvent) error {
	return p.enqueue(ctx, event.OrderID, "payment.failed", p.paymentFailedSubj, event)
}

func (p *OutboxPublisher) PublishRefundSucceeded(ctx context.Context, event app.RefundSucceededEvent) error {
	return p.enqueue(ctx, event.OrderID, "payment.refund_succeeded", p.refundSucceededSubj, event)
}

func (p *OutboxPublisher) PublishRefundFailed(ctx context.Context, event app.RefundFailedEvent) error {
	return p.enqueue(ctx, event.OrderID, "payment.refund_failed", p.refundFailedSubj, event)
}

func (p *OutboxPublisher) enqueue(ctx context.Context, orderID, eventType, subject string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal payment outbox: %w", err)
	}
	aggregateID, err := uuid.Parse(orderID)
	if err != nil {
		aggregateID = uuid.NewV7()
	}
	const query = `
		INSERT INTO outbox (aggregate_id, event_type, subject, payload)
		VALUES ($1, $2, $3, $4)`
	if _, err := p.pool.Exec(ctx, query, aggregateID, eventType, subject, payload); err != nil {
		return fmt.Errorf("insert payment outbox: %w", err)
	}
	return nil
}
