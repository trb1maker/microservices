package eventpublisher

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/payment-service/app"
	"github.com/trb1maker/microservices/internal/platform/natsx"
)

type NATSEventPublisher struct {
	client               *natsx.Client
	paymentSucceededSubj string
	paymentFailedSubj    string
	refundSucceededSubj  string
	refundFailedSubj     string
}

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
	return p.publish(ctx, p.paymentSucceededSubj, event, event.OrderID)
}

func (p *NATSEventPublisher) PublishPaymentFailed(ctx context.Context, event app.PaymentFailedEvent) error {
	return p.publish(ctx, p.paymentFailedSubj, event, event.OrderID)
}

func (p *NATSEventPublisher) PublishRefundSucceeded(ctx context.Context, event app.RefundSucceededEvent) error {
	return p.publish(ctx, p.refundSucceededSubj, event, event.OrderID)
}

func (p *NATSEventPublisher) PublishRefundFailed(ctx context.Context, event app.RefundFailedEvent) error {
	return p.publish(ctx, p.refundFailedSubj, event, event.OrderID)
}

func (p *NATSEventPublisher) publish(ctx context.Context, subject string, event any, orderID string) error {
	headers := nats.Header{}
	if orderID != "" {
		headers.Set(natsx.HeaderOrderID, orderID)
	}
	if err := p.client.PublishJSON(ctx, subject, event, headers); err != nil {
		return fmt.Errorf("publish payment event: %w", err)
	}
	return nil
}
