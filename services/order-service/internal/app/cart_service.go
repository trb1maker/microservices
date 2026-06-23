package app

import (
	"context"
	"fmt"
	"order-service/internal/domain"
)

type CartService struct {
	carts CartRepository
}

func NewCartService(carts CartRepository) *CartService {
	return &CartService{carts: carts}
}

func (s *CartService) AddItem(
	ctx context.Context,
	userID domain.UserID,
	item domain.OrderItem,
) (*domain.Cart, error) {
	cart, err := s.getOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	if err := cart.AddItem(item); err != nil {
		return nil, fmt.Errorf("add item to cart: %w", err)
	}

	if err := s.carts.Save(ctx, cart); err != nil {
		return nil, fmt.Errorf("save cart: %w", err)
	}

	return cart, nil
}

func (s *CartService) GetCart(ctx context.Context, userID domain.UserID) (*domain.Cart, error) {
	return s.getOrCreateCart(ctx, userID)
}

func (s *CartService) RemoveItem(
	ctx context.Context,
	userID domain.UserID,
	productID domain.ProductID,
) (*domain.Cart, error) {
	cart, err := s.getOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	if err := cart.RemoveItem(productID); err != nil {
		return nil, fmt.Errorf("remove item from cart: %w", err)
	}

	if err := s.carts.Save(ctx, cart); err != nil {
		return nil, fmt.Errorf("save cart: %w", err)
	}

	return cart, nil
}

func (s *CartService) getOrCreateCart(ctx context.Context, userID domain.UserID) (*domain.Cart, error) {
	cart, err := s.carts.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get cart: %w", err)
	}

	if cart != nil {
		return cart, nil
	}

	cart, err = domain.NewCart(userID)
	if err != nil {
		return nil, fmt.Errorf("create cart: %w", err)
	}

	if err := s.carts.Save(ctx, cart); err != nil {
		return nil, fmt.Errorf("save cart: %w", err)
	}

	return cart, nil
}
