package memory

import (
	"context"
	"sync"

	"github.com/trb1maker/microservices/services/order-service/internal/domain"
)

type CartRepository struct {
	mu    sync.RWMutex
	carts map[domain.UserID]*domain.Cart
}

func NewCartRepository() *CartRepository {
	return &CartRepository{
		carts: make(map[domain.UserID]*domain.Cart),
	}
}

func (r *CartRepository) Get(_ context.Context, userID domain.UserID) (*domain.Cart, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cart, ok := r.carts[userID]
	if !ok {
		return nil, nil
	}

	return cart, nil
}

func (r *CartRepository) Save(_ context.Context, cart *domain.Cart) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.carts[cart.UserID()] = cart

	return nil
}
