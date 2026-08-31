package app

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/trb1maker/microservices/internal/order-service/domain"
)

type CartService struct {
	carts  CartRepository
	events CartEventPublisher
}

func NewCartService(carts CartRepository, publishers ...CartEventPublisher) *CartService {
	events := CartEventPublisher(NewNoopEventPublisher())
	if len(publishers) > 0 && publishers[0] != nil {
		events = publishers[0]
	}
	return &CartService{carts: carts, events: events}
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

	orderID := cart.EnsurePendingOrderID()

	if err := s.carts.Save(ctx, cart); err != nil {
		return nil, fmt.Errorf("save cart: %w", err)
	}

	if err := s.events.PublishReserveItems(ctx, ReserveItems{
		UserID:    uuid.UUID(userID).String(),
		ProductID: uuid.UUID(item.ProductID()).String(),
		Quantity:  int(item.Quantity()),
	}, uuid.UUID(orderID).String()); err != nil {
		return nil, fmt.Errorf("publish reserve items: %w", err)
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

	index := -1
	for i, item := range cart.Items() {
		if item.ProductID() == productID {
			index = i
			break
		}
	}
	if index == -1 {
		return nil, domain.ErrItemNotFound
	}

	item := cart.Items()[index]
	if item.ReservationStatus() == domain.ReservationStatusReserved && cart.PendingOrderID() != (domain.OrderID{}) {
		if err := s.events.PublishReleaseReservation(ctx, ReleaseReservation{
			UserID:    uuid.UUID(userID).String(),
			OrderID:   uuid.UUID(cart.PendingOrderID()).String(),
			ProductID: uuid.UUID(productID).String(),
			Quantity:  int(item.Quantity()),
		}); err != nil {
			return nil, fmt.Errorf("publish release reservation: %w", err)
		}
	}

	if err := cart.RemoveItem(productID); err != nil {
		return nil, fmt.Errorf("remove item from cart: %w", err)
	}

	if err := s.carts.Save(ctx, cart); err != nil {
		return nil, fmt.Errorf("save cart: %w", err)
	}

	return cart, nil
}

func (s *CartService) HandleItemsReserved(ctx context.Context, event ItemsReserved) error {
	userID, err := uuid.Parse(event.UserID)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}
	productID, err := uuid.Parse(event.ProductID)
	if err != nil {
		return fmt.Errorf("parse product id: %w", err)
	}

	cart, err := s.carts.Get(ctx, domain.UserID(userID))
	if err != nil {
		return fmt.Errorf("get cart: %w", err)
	}
	if cart == nil {
		return nil
	}

	if err := cart.MarkItemReserved(domain.ProductID(productID)); err != nil {
		return fmt.Errorf("mark item reserved: %w", err)
	}

	if err := s.carts.Save(ctx, cart); err != nil {
		return fmt.Errorf("save cart: %w", err)
	}

	return nil
}

func (s *CartService) HandleReservationFailed(ctx context.Context, event ReservationFailed) error {
	userID, err := uuid.Parse(event.UserID)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}
	productID, err := uuid.Parse(event.ProductID)
	if err != nil {
		return fmt.Errorf("parse product id: %w", err)
	}

	cart, err := s.carts.Get(ctx, domain.UserID(userID))
	if err != nil {
		return fmt.Errorf("get cart: %w", err)
	}
	if cart == nil {
		return nil
	}

	if err := cart.MarkItemFailed(domain.ProductID(productID)); err != nil {
		if errors.Is(err, domain.ErrItemNotFound) {
			// Cart is cleared after checkout; order service handles compensation.
			return nil
		}
		return fmt.Errorf("mark item failed: %w", err)
	}

	if err := s.carts.Save(ctx, cart); err != nil {
		return fmt.Errorf("save cart: %w", err)
	}

	return nil
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
