package eventpublisher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/payment-service/app"
)

// NATSEventPublisher publishes payment events to NATS.
type NATSEventPublisher struct {
	conn                 *nats.Conn
	paymentSucceededSubj string
	paymentFailedSubj    string
	refundSucceededSubj  string
	refundFailedSubj     string
}

// NewNATSEventPublisher creates a new NATSEventPublisher.
func NewNATSEventPublisher(
	conn *nats.Conn,
	paymentSucceededSubj,
	paymentFailedSubj,
	refundSucceededSubj,
	refundFailedSubj string,
) *NATSEventPublisher {
	return &NATSEventPublisher{
		conn:                 conn,
		paymentSucceededSubj: paymentSucceededSubj,
		paymentFailedSubj:    paymentFailedSubj,
		refundSucceededSubj:  refundSucceededSubj,
		refundFailedSubj:     refundFailedSubj,
	}
}

func (p *NATSEventPublisher) PublishPaymentSucceeded(ctx context.Context, event app.PaymentSucceededEvent) error {
	return p.publish(ctx, p.paymentSucceededSubj, event)
}

func (p *NATSEventPublisher) PublishPaymentFailed(ctx context.Context, event app.PaymentFailedEvent) error {
	return p.publish(ctx, p.paymentFailedSubj, event)
}

func (p *NATSEventPublisher) PublishRefundSucceeded(ctx context.Context, event app.RefundSucceededEvent) error {
	return p.publish(ctx, p.refundSucceededSubj, event)
}

func (p *NATSEventPublisher) PublishRefundFailed(ctx context.Context, event app.RefundFailedEvent) error {
	return p.publish(ctx, p.refundFailedSubj, event)
}

func (p *NATSEventPublisher) publish(_ context.Context, subject string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if err := p.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}
