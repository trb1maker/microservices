package app

import (
	"context"
)

type NoopEventPublisher struct{}

func NewNoopEventPublisher() *NoopEventPublisher {
	return &NoopEventPublisher{}
}

func (NoopEventPublisher) PublishReserveItems(context.Context, ReserveItems) error {
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
