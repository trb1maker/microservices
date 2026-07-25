package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/trb1maker/microservices/internal/store-service/domain"
)

var (
	errInvalidQuantity      = fmt.Errorf("invalid quantity")
	errInsufficientReserved = fmt.Errorf("insufficient reserved stock")
)

// StoreService implements the store business logic.
type StoreService struct {
	products ProductRepository
	stocks   StockRepository
	events   EventPublisher
}

// NewStoreService creates a new StoreService.
func NewStoreService(products ProductRepository, stocks StockRepository, events EventPublisher) *StoreService {
	return &StoreService{
		products: products,
		stocks:   stocks,
		events:   events,
	}
}

// ReserveItemsRequest holds the parameters for a reservation.
type ReserveItemsRequest struct {
	OrderID   string
	UserID    string
	ProductID string
	Quantity  int
}

// ReserveItems reserves items in stock.
func (s *StoreService) ReserveItems(ctx context.Context, req ReserveItemsRequest) error {
	if req.Quantity <= 0 {
		return fmt.Errorf("%w: %d", errInvalidQuantity, req.Quantity)
	}

	// Verify product exists
	_, err := s.products.Get(ctx, domain.ProductID(req.ProductID))
	if err != nil {
		s.publishReservationFailed(ctx, req.OrderID, req.UserID, req.ProductID, req.Quantity, "product not found")
		return fmt.Errorf("get product: %w", err)
	}

	// Get stock
	stock, err := s.stocks.Get(ctx, domain.ProductID(req.ProductID))
	if err != nil {
		s.publishReservationFailed(ctx, req.OrderID, req.UserID, req.ProductID, req.Quantity, "stock not found")
		return fmt.Errorf("get stock: %w", err)
	}

	// Check availability
	if !stock.CanReserve(req.Quantity) {
		s.publishReservationFailed(ctx, req.OrderID, req.UserID, req.ProductID, req.Quantity, "insufficient stock")
		slog.Warn("reservation failed: insufficient stock",
			slog.String("product_id", req.ProductID),
			slog.Int("available", stock.Available),
			slog.Int("requested", req.Quantity),
		)
		return nil
	}

	// Reserve
	stock.Reserve(req.Quantity)
	if err := s.stocks.Update(ctx, stock); err != nil {
		s.publishReservationFailed(ctx, req.OrderID, req.UserID, req.ProductID, req.Quantity, "update failed")
		return fmt.Errorf("update stock: %w", err)
	}

	s.publishItemsReserved(ctx, req.OrderID, req.UserID, req.ProductID, req.Quantity)

	slog.Info("items reserved",
		slog.String("order_id", req.OrderID),
		slog.String("product_id", req.ProductID),
		slog.Int("quantity", req.Quantity),
	)

	return nil
}

// ConfirmOrderRequest holds the parameters for order confirmation.
type ConfirmOrderRequest struct {
	OrderID   string
	UserID    string
	ProductID string
	Quantity  int
}

// ConfirmOrder confirms an order (deducts reserved stock).
func (s *StoreService) ConfirmOrder(ctx context.Context, req ConfirmOrderRequest) error {
	stock, err := s.stocks.Get(ctx, domain.ProductID(req.ProductID))
	if err != nil {
		return fmt.Errorf("get stock: %w", err)
	}

	if stock.Reserved < req.Quantity {
		s.publishReservationFailed(ctx, req.OrderID, req.UserID, req.ProductID, req.Quantity, "reservation not found")
		return fmt.Errorf("%w: %d < %d", errInsufficientReserved, stock.Reserved, req.Quantity)
	}

	stock.Confirm(req.Quantity)
	if err := s.stocks.Update(ctx, stock); err != nil {
		return fmt.Errorf("update stock: %w", err)
	}

	slog.Info("order confirmed",
		slog.String("order_id", req.OrderID),
		slog.String("product_id", req.ProductID),
		slog.Int("quantity", req.Quantity),
	)

	s.publishOrderConfirmed(ctx, req.OrderID, req.UserID)

	return nil
}

func (s *StoreService) publishOrderConfirmed(ctx context.Context, orderID, userID string) {
	event := OrderConfirmedEvent{
		OrderID:   orderID,
		UserID:    userID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.events.PublishOrderConfirmed(ctx, event); err != nil {
		slog.Error("failed to publish order confirmed event", slog.Any("error", err))
	}
}

// ReleaseReservationRequest holds the parameters for releasing a reservation.
type ReleaseReservationRequest struct {
	OrderID   string
	UserID    string
	ProductID string
	Quantity  int
}

// ReleaseReservation releases a reservation (cancels the hold).
func (s *StoreService) ReleaseReservation(ctx context.Context, req ReleaseReservationRequest) error {
	stock, err := s.stocks.Get(ctx, domain.ProductID(req.ProductID))
	if err != nil {
		return fmt.Errorf("get stock: %w", err)
	}

	if stock.Reserved < req.Quantity {
		return fmt.Errorf("%w to release: %d < %d", errInsufficientReserved, stock.Reserved, req.Quantity)
	}

	stock.Release(req.Quantity)
	if err := s.stocks.Update(ctx, stock); err != nil {
		return fmt.Errorf("update stock: %w", err)
	}

	s.publishReservationReleased(ctx, req.OrderID, req.UserID, req.ProductID, req.Quantity)

	slog.Info("reservation released",
		slog.String("order_id", req.OrderID),
		slog.String("product_id", req.ProductID),
		slog.Int("quantity", req.Quantity),
	)

	return nil
}

func (s *StoreService) publishItemsReserved(ctx context.Context, orderID, userID, productID string, quantity int) {
	event := ItemsReservedEvent{
		OrderID:   orderID,
		UserID:    userID,
		ProductID: productID,
		Quantity:  quantity,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.events.PublishItemsReserved(ctx, event); err != nil {
		slog.Error("failed to publish items reserved event", slog.Any("error", err))
	}
}

func (s *StoreService) publishReservationFailed(ctx context.Context, orderID, userID, productID string, quantity int, reason string) {
	event := ReservationFailedEvent{
		OrderID:   orderID,
		UserID:    userID,
		ProductID: productID,
		Quantity:  quantity,
		Reason:    reason,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.events.PublishReservationFailed(ctx, event); err != nil {
		slog.Error("failed to publish reservation failed event", slog.Any("error", err))
	}
}

func (s *StoreService) publishReservationReleased(ctx context.Context, orderID, userID, productID string, quantity int) {
	event := ReservationReleasedEvent{
		OrderID:   orderID,
		UserID:    userID,
		ProductID: productID,
		Quantity:  quantity,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.events.PublishReservationReleased(ctx, event); err != nil {
		slog.Error("failed to publish reservation released event", slog.Any("error", err))
	}
}
