package memory

import (
	"context"
	"fmt"
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

	return cloneCart(cart)
}

func (r *CartRepository) Save(_ context.Context, cart *domain.Cart) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cloned, err := cloneCart(cart)
	if err != nil {
		return err
	}

	r.carts[cart.UserID()] = cloned

	return nil
}

func cloneCart(cart *domain.Cart) (*domain.Cart, error) {
	cloned, err := domain.ReconstituteCart(cart.UserID(), cart.UpdatedAt(), cart.Items()...)
	if err != nil {
		return nil, fmt.Errorf("reconstitute cart: %w", err)
	}

	return cloned, nil
}
