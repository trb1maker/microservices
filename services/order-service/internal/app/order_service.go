package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	"github.com/google/uuid"
)

type OrderService struct {
	carts  CartRepository
	orders OrderRepository
	events EventPublisher
}

func NewOrderService(
	carts CartRepository,
	orders OrderRepository,
	events EventPublisher,
) *OrderService {
	return &OrderService{
		carts:  carts,
		orders: orders,
		events: events,
	}
}

func (s *OrderService) Checkout(
	ctx context.Context,
	userID domain.UserID,
	deliveryAddress string,
	now time.Time,
) (*domain.Order, error) {
	if strings.TrimSpace(deliveryAddress) == "" {
		return nil, ErrDeliveryAddressRequired
	}

	cart, err := s.carts.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get cart: %w", err)
	}

	if cart == nil {
		return nil, domain.ErrEmptyCart
	}

	order, err := cart.Checkout(domain.OrderID(uuid.New()), deliveryAddress, now)
	if err != nil {
		return nil, fmt.Errorf("checkout cart: %w", err)
	}

	if err := s.orders.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	cart.Clear()
	if err := s.carts.Save(ctx, cart); err != nil {
		return nil, fmt.Errorf("save cart: %w", err)
	}

	if err := s.publishOrderCreated(ctx, order); err != nil {
		if rollbackErr := s.rollbackCheckout(ctx, userID, order); rollbackErr != nil {
			return nil, fmt.Errorf("publish order created: %w (rollback failed: %w)", err, rollbackErr)
		}

		return nil, fmt.Errorf("publish order created: %w", err)
	}

	return order, nil
}

func (s *OrderService) rollbackCheckout(ctx context.Context, userID domain.UserID, order *domain.Order) error {
	if err := s.orders.Delete(ctx, order.OrderID()); err != nil {
		return fmt.Errorf("delete order: %w", err)
	}

	restoredCart, err := domain.ReconstituteCart(userID, time.Now(), order.Items()...)
	if err != nil {
		return fmt.Errorf("reconstitute cart: %w", err)
	}

	if err := s.carts.Save(ctx, restoredCart); err != nil {
		return fmt.Errorf("restore cart: %w", err)
	}

	return nil
}

func (s *OrderService) publishOrderCreated(ctx context.Context, order *domain.Order) error {
	if err := s.events.PublishOrderCreated(ctx, OrderCreated{
		OrderID:    uuid.UUID(order.OrderID()).String(),
		UserID:     uuid.UUID(order.UserID()).String(),
		TotalPrice: order.TotalPrice(),
	}); err != nil {
		return fmt.Errorf("publish order created: %w", err)
	}

	return nil
}

func (s *OrderService) GetOrder(ctx context.Context, userID domain.UserID, orderID domain.OrderID) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}

	if order == nil || order.UserID() != userID {
		return nil, ErrOrderNotFound
	}

	return order, nil
}

func (s *OrderService) CancelOrder(
	ctx context.Context,
	userID domain.UserID,
	orderID domain.OrderID,
	now time.Time,
) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}

	if order == nil || order.UserID() != userID {
		return nil, ErrOrderNotFound
	}

	releaseReservation := order.Status() == domain.OrderStatusPending || order.Status() == domain.OrderStatusReserved

	if err := order.Cancel(now); err != nil {
		return nil, fmt.Errorf("cancel order: %w", err)
	}

	if err := s.orders.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	if err := s.publishOrderCancelled(ctx, order); err != nil {
		return nil, fmt.Errorf("publish order cancelled: %w", err)
	}

	if releaseReservation {
		if err := s.events.PublishReleaseReservation(ctx, ReleaseReservation{
			UserID:  uuid.UUID(order.UserID()).String(),
			OrderID: uuid.UUID(order.OrderID()).String(),
		}); err != nil {
			return nil, fmt.Errorf("publish release reservation: %w", err)
		}
	}

	return order, nil
}

func (s *OrderService) publishOrderCancelled(ctx context.Context, order *domain.Order) error {
	if err := s.events.PublishOrderCancelled(ctx, OrderCancelled{
		OrderID: uuid.UUID(order.OrderID()).String(),
		UserID:  uuid.UUID(order.UserID()).String(),
	}); err != nil {
		return fmt.Errorf("publish order cancelled: %w", err)
	}

	return nil
}
