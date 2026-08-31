package eventpublisher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trb1maker/microservices/internal/payment-service/app"
	"github.com/trb1maker/microservices/internal/platform/natsx"
)

// NATSEventPublisher publishes payment events to JetStream.
type NATSEventPublisher struct {
	client               *natsx.Client
	paymentSucceededSubj string
	paymentFailedSubj    string
	refundSucceededSubj  string
	refundFailedSubj     string
}

// NewNATSEventPublisher creates a new NATSEventPublisher.
func NewNATSEventPublisher(
	client *natsx.Client,
	paymentSucceededSubj,
	paymentFailedSubj,
	refundSucceededSubj,
	refundFailedSubj string,
) *NATSEventPublisher {
	return &NATSEventPublisher{
		client:               client,
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

func (p *NATSEventPublisher) publish(ctx context.Context, subject string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if err := p.client.Publish(ctx, subject, data); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}
