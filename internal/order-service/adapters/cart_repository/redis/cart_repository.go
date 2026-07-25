package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/trb1maker/microservices/internal/order-service/domain"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const cartKeyPrefix = "cart:"

type CartRepository struct {
	client *goredis.Client
}

func NewCartRepository(client *goredis.Client) *CartRepository {
	return &CartRepository{client: client}
}

type cartDTO struct {
	UserID         string    `json:"user_id"`
	PendingOrderID string    `json:"pending_order_id,omitempty"`
	Items          []itemDTO `json:"items"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type itemDTO struct {
	ProductID         string `json:"product_id"`
	Quantity          int64  `json:"quantity"`
	UnitPrice         int64  `json:"unit_price"`
	TotalPrice        int64  `json:"total_price"`
	ReservationStatus string `json:"reservation_status"`
}

func (r *CartRepository) Get(ctx context.Context, userID domain.UserID) (*domain.Cart, error) {
	key := cartKey(userID)

	data, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("get cart: %w", err)
	}

	var dto cartDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fmt.Errorf("unmarshal cart: %w", err)
	}

	cart, err := fromDTO(dto)
	if err != nil {
		return nil, err
	}

	return cart, nil
}

func (r *CartRepository) Save(ctx context.Context, cart *domain.Cart) error {
	dto := toDTO(cart)

	data, err := json.Marshal(dto)
	if err != nil {
		return fmt.Errorf("marshal cart: %w", err)
	}

	if err := r.client.Set(ctx, cartKey(cart.UserID()), data, 0).Err(); err != nil {
		return fmt.Errorf("set cart: %w", err)
	}

	return nil
}

func (r *CartRepository) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	return nil
}

func cartKey(userID domain.UserID) string {
	return cartKeyPrefix + uuid.UUID(userID).String()
}

func toDTO(cart *domain.Cart) cartDTO {
	items := make([]itemDTO, 0, len(cart.Items()))
	for _, item := range cart.Items() {
		items = append(items, itemDTO{
			ProductID:         uuid.UUID(item.ProductID()).String(),
			Quantity:          item.Quantity(),
			UnitPrice:         item.UnitPrice(),
			TotalPrice:        item.TotalPrice(),
			ReservationStatus: string(item.ReservationStatus()),
		})
	}

	dto := cartDTO{
		UserID:    uuid.UUID(cart.UserID()).String(),
		Items:     items,
		UpdatedAt: cart.UpdatedAt(),
	}
	if cart.PendingOrderID() != (domain.OrderID{}) {
		dto.PendingOrderID = uuid.UUID(cart.PendingOrderID()).String()
	}
	return dto
}

func fromDTO(dto cartDTO) (*domain.Cart, error) {
	userID, err := uuid.Parse(dto.UserID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	var pendingOrderID domain.OrderID
	if dto.PendingOrderID != "" {
		parsed, err := uuid.Parse(dto.PendingOrderID)
		if err != nil {
			return nil, fmt.Errorf("parse pending order id: %w", err)
		}
		pendingOrderID = domain.OrderID(parsed)
	}

	items := make([]domain.OrderItem, 0, len(dto.Items))
	for _, itemDTO := range dto.Items {
		productID, err := uuid.Parse(itemDTO.ProductID)
		if err != nil {
			return nil, fmt.Errorf("parse product id: %w", err)
		}

		item, err := domain.ReconstituteOrderItem(
			domain.ProductID(productID),
			itemDTO.Quantity,
			itemDTO.UnitPrice,
			domain.ReservationStatus(itemDTO.ReservationStatus),
		)
		if err != nil {
			return nil, fmt.Errorf("build item: %w", err)
		}

		items = append(items, *item)
	}

	cart, err := domain.ReconstituteCart(domain.UserID(userID), pendingOrderID, dto.UpdatedAt, items...)
	if err != nil {
		return nil, fmt.Errorf("build cart: %w", err)
	}

	return cart, nil
}
