package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/trb1maker/microservices/services/order-service/internal/domain"

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
	UserID    string    `json:"user_id"`
	Items     []itemDTO `json:"items"`
	UpdatedAt time.Time `json:"updated_at"`
}

type itemDTO struct {
	ProductID  string `json:"product_id"`
	Quantity   int64  `json:"quantity"`
	UnitPrice  int64  `json:"unit_price"`
	TotalPrice int64  `json:"total_price"`
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

	return fromDTO(dto)
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
			ProductID:  uuid.UUID(item.ProductID()).String(),
			Quantity:   item.Quantity(),
			UnitPrice:  item.UnitPrice(),
			TotalPrice: item.TotalPrice(),
		})
	}

	return cartDTO{
		UserID:    uuid.UUID(cart.UserID()).String(),
		Items:     items,
		UpdatedAt: cart.UpdatedAt(),
	}
}

func fromDTO(dto cartDTO) (*domain.Cart, error) {
	userID, err := uuid.Parse(dto.UserID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	items := make([]domain.OrderItem, 0, len(dto.Items))
	for _, itemDTO := range dto.Items {
		productID, err := uuid.Parse(itemDTO.ProductID)
		if err != nil {
			return nil, fmt.Errorf("parse product id: %w", err)
		}

		item, err := domain.NewOrderItem(domain.ProductID(productID), itemDTO.Quantity, itemDTO.UnitPrice)
		if err != nil {
			return nil, fmt.Errorf("build item: %w", err)
		}

		items = append(items, *item)
	}

	cart, err := domain.NewCart(domain.UserID(userID), items...)
	if err != nil {
		return nil, fmt.Errorf("build cart: %w", err)
	}

	return cart, nil
}
