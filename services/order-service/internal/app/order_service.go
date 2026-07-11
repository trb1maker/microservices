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
	events OrderEventPublisher
}

func NewOrderService(
	carts CartRepository,
	orders OrderRepository,
	events OrderEventPublisher,
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

	order, err := cart.Checkout(domain.OrderID(uuid.New()), now)
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
		return nil, fmt.Errorf("publish order created: %w", err)
	}

	return order, nil
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

func (s *OrderService) GetOrder(ctx context.Context, orderID domain.OrderID) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}

	if order == nil {
		return nil, ErrOrderNotFound
	}

	return order, nil
}

func (s *OrderService) CancelOrder(ctx context.Context, orderID domain.OrderID, now time.Time) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}

	if order == nil {
		return nil, ErrOrderNotFound
	}

	if err := order.Cancel(now); err != nil {
		return nil, fmt.Errorf("cancel order: %w", err)
	}

	if err := s.orders.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("save order: %w", err)
	}

	return order, nil
}
