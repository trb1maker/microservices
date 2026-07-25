package app

import (
	"context"

	"github.com/trb1maker/microservices/internal/store-service/domain"
)

// ProductRepository defines the interface for product catalog storage.
type ProductRepository interface {
	Get(ctx context.Context, id domain.ProductID) (*domain.Product, error)
}

// StockRepository defines the interface for stock storage.
type StockRepository interface {
	Get(ctx context.Context, productID domain.ProductID) (*domain.Stock, error)
	// Update atomically updates the stock. Returns error on conflict.
	Update(ctx context.Context, stock *domain.Stock) error
}

// EventPublisher defines the interface for publishing store events.
type EventPublisher interface {
	PublishItemsReserved(ctx context.Context, event ItemsReservedEvent) error
	PublishReservationFailed(ctx context.Context, event ReservationFailedEvent) error
	PublishOrderConfirmed(ctx context.Context, event OrderConfirmedEvent) error
	PublishReservationReleased(ctx context.Context, event ReservationReleasedEvent) error
}

// ItemsReservedEvent is published after successful reservation.
type ItemsReservedEvent struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Timestamp string `json:"timestamp"`
}

// ReservationFailedEvent is published when reservation fails.
type ReservationFailedEvent struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// OrderConfirmedEvent is published after successful order confirmation.
type OrderConfirmedEvent struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	Timestamp string `json:"timestamp"`
}

// ReservationReleasedEvent is published after reservation is released.
type ReservationReleasedEvent struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Timestamp string `json:"timestamp"`
}
