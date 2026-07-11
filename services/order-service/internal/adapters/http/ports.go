package http

import (
	"context"
	"net/http"
	"time"

	"github.com/trb1maker/microservices/pkg/middleware"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"
)

type CartService interface {
	AddItem(ctx context.Context, userID domain.UserID, item domain.OrderItem) (*domain.Cart, error)
	GetCart(ctx context.Context, userID domain.UserID) (*domain.Cart, error)
	RemoveItem(ctx context.Context, userID domain.UserID, productID domain.ProductID) (*domain.Cart, error)
}

type OrderService interface {
	Checkout(ctx context.Context, userID domain.UserID, deliveryAddress string, now time.Time) (*domain.Order, error)
	GetOrder(ctx context.Context, userID domain.UserID, orderID domain.OrderID) (*domain.Order, error)
	CancelOrder(ctx context.Context, userID domain.UserID, orderID domain.OrderID, now time.Time) (*domain.Order, error)
}

type ReadinessChecker interface {
	Check(ctx context.Context) (ready bool, checks map[string]string)
}

// HTTPExporter exposes the Prometheus scrape endpoint.
type HTTPExporter interface {
	Handler() http.Handler
}

// HTTPMetrics exposes scrape endpoint and instruments handlers.
type HTTPMetrics interface {
	HTTPExporter
	middleware.HTTPInstrumenter
}
