package app

import (
	"context"
	"uuid"

	"github.com/trb1maker/microservices/internal/order-service/domain"
)

type NoopEventPublisher struct{}

func NewNoopEventPublisher() *NoopEventPublisher {
	return &NoopEventPublisher{}
}

func (NoopEventPublisher) PublishReserveItems(context.Context, ReserveItems, string) error {
	return nil
}

func (NoopEventPublisher) PublishReleaseReservation(context.Context, ReleaseReservation) error {
	return nil
}

func (NoopEventPublisher) PublishOrderCreated(context.Context, OrderCreated) error {
	return nil
}

func (NoopEventPublisher) PublishConfirmOrder(context.Context, ConfirmOrder) error {
	return nil
}

func (NoopEventPublisher) PublishOrderFinalized(context.Context, OrderFinalized) error {
	return nil
}

func (NoopEventPublisher) PublishOrderCancelled(context.Context, OrderCancelled) error {
	return nil
}

type NoopPaymentClient struct{}

func NewNoopPaymentClient() *NoopPaymentClient {
	return &NoopPaymentClient{}
}

func (NoopPaymentClient) Charge(context.Context, string, string, int64) (string, bool, string, error) {
	return uuid.NewV7().String(), true, "noop payment", nil
}

func (NoopPaymentClient) Refund(context.Context, string, string, int64, string) (string, bool, string, error) {
	return uuid.NewV7().String(), true, "noop refund", nil
}

type NoopStatusNotifier struct{}

func (NoopStatusNotifier) NotifyOrderStatus(*domain.Order) {}
