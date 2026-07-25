package memory

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/trb1maker/microservices/internal/order-service/domain"
)

type OrderRepository struct {
	mu     sync.RWMutex
	orders map[domain.OrderID]*domain.Order
}

func NewOrderRepository() *OrderRepository {
	return &OrderRepository{
		orders: make(map[domain.OrderID]*domain.Order),
	}
}

func (r *OrderRepository) Get(_ context.Context, orderID domain.OrderID) (*domain.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	order, ok := r.orders[orderID]
	if !ok {
		return nil, nil
	}

	return cloneOrder(order)
}

func (r *OrderRepository) ListByUser(_ context.Context, userID domain.UserID, limit, offset int) ([]*domain.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	orders := make([]*domain.Order, 0)
	for _, order := range r.orders {
		if order.UserID() != userID {
			continue
		}
		cloned, err := cloneOrder(order)
		if err != nil {
			return nil, err
		}
		orders = append(orders, cloned)
	}
	slices.SortFunc(orders, func(a, b *domain.Order) int {
		return b.CreatedAt().Compare(a.CreatedAt())
	})
	if offset >= len(orders) {
		return []*domain.Order{}, nil
	}
	orders = orders[offset:]
	if limit < len(orders) {
		orders = orders[:limit]
	}
	return orders, nil
}

func (r *OrderRepository) Save(_ context.Context, order *domain.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cloned, err := cloneOrder(order)
	if err != nil {
		return err
	}

	r.orders[order.OrderID()] = cloned

	return nil
}

func (r *OrderRepository) Delete(_ context.Context, orderID domain.OrderID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.orders, orderID)

	return nil
}

func (r *OrderRepository) CountActiveOrders(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, order := range r.orders {
		status := order.Status()
		if status != domain.OrderStatusConfirmed && status != domain.OrderStatusCancelled {
			count++
		}
	}

	return count, nil
}

func cloneOrder(order *domain.Order) (*domain.Order, error) {
	cloned, err := domain.ReconstituteOrder(
		order.OrderID(),
		order.UserID(),
		order.Status(),
		order.PaymentID(),
		order.DeliveryAddress(),
		order.CreatedAt(),
		order.UpdatedAt(),
		order.StatusHistory(),
		order.Items()...,
	)
	if err != nil {
		return nil, fmt.Errorf("clone order: %w", err)
	}

	return cloned, nil
}
