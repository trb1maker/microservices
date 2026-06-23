package app

import (
	"context"
	"fmt"
	"order-service/internal/domain"
	"strings"
	"time"

	"github.com/google/uuid"
)

type OrderService struct {
	carts  CartRepository
	orders OrderRepository
}

func NewOrderService(carts CartRepository, orders OrderRepository) *OrderService {
	return &OrderService{carts: carts, orders: orders}
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

	return order, nil
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
