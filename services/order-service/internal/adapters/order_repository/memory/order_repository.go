package memory

import (
	"context"
	"sync"

	"github.com/trb1maker/microservices/services/order-service/internal/domain"
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

	return order, nil
}

func (r *OrderRepository) Save(_ context.Context, order *domain.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.orders[order.OrderID()] = order

	return nil
}
