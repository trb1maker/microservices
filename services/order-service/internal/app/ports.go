package app

import (
	"context"

	"github.com/trb1maker/microservices/services/order-service/internal/domain"
)

type CartRepository interface {
	Get(ctx context.Context, userID domain.UserID) (*domain.Cart, error)
	Save(ctx context.Context, cart *domain.Cart) error
}

type OrderRepository interface {
	Get(ctx context.Context, orderID domain.OrderID) (*domain.Order, error)
	Save(ctx context.Context, order *domain.Order) error
}

type CartEventPublisher interface {
	PublishReserveItems(ctx context.Context, event ReserveItems) error
	PublishReleaseReservation(ctx context.Context, event ReleaseReservation) error
}

type OrderEventPublisher interface {
	PublishOrderCreated(ctx context.Context, event OrderCreated) error
	PublishConfirmOrder(ctx context.Context, event ConfirmOrder) error
	PublishOrderFinalized(ctx context.Context, event OrderFinalized) error
	PublishOrderCancelled(ctx context.Context, event OrderCancelled) error
}
