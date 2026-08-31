package app

import (
	"context"

	"github.com/trb1maker/microservices/internal/platform/events"
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

type ReservationStore interface {
	Seen(ctx context.Context, orderID, operation string) (bool, error)
	Mark(ctx context.Context, orderID, operation string) error
}

// EventPublisher defines the interface for publishing store events.
type EventPublisher interface {
	PublishItemsReserved(ctx context.Context, event ItemsReservedEvent) error
	PublishReservationFailed(ctx context.Context, event ReservationFailedEvent) error
	PublishOrderConfirmed(ctx context.Context, event OrderConfirmedEvent) error
	PublishReservationReleased(ctx context.Context, event ReservationReleasedEvent) error
}

type (
	ItemsReservedEvent       = events.ItemsReserved
	ReservationFailedEvent   = events.ReservationFailed
	OrderConfirmedEvent      = events.OrderConfirmed
	ReservationReleasedEvent = events.ReservationReleased
)
