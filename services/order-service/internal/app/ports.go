package app

import (
	"context"
	"order-service/internal/domain"
)

type CartRepository interface {
	Get(ctx context.Context, userID domain.UserID) (*domain.Cart, error)
	Save(ctx context.Context, cart *domain.Cart) error
}

type OrderRepository interface {
	Get(ctx context.Context, orderID domain.OrderID) (*domain.Order, error)
	Save(ctx context.Context, order *domain.Order) error
}
