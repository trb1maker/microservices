package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

type orderRow struct {
	userID          uuid.UUID
	status          string
	totalPrice      int64
	paymentID       *uuid.UUID
	deliveryAddress string
	createdAt       time.Time
	updatedAt       time.Time
}

func (r *OrderRepository) Get(ctx context.Context, orderID domain.OrderID) (*domain.Order, error) {
	row, err := r.fetchOrderRow(ctx, orderID)
	if err != nil || row == nil {
		return nil, err
	}

	items, err := r.fetchOrderItems(ctx, orderID)
	if err != nil {
		return nil, err
	}

	return buildOrder(orderID, row, items)
}

func (r *OrderRepository) fetchOrderRow(ctx context.Context, orderID domain.OrderID) (*orderRow, error) {
	const orderQuery = `
		SELECT user_id, status, total_price, payment_id, delivery_address, created_at, updated_at
		FROM orders
		WHERE order_id = $1`

	var row orderRow

	err := r.pool.QueryRow(ctx, orderQuery, uuid.UUID(orderID)).Scan(
		&row.userID,
		&row.status,
		&row.totalPrice,
		&row.paymentID,
		&row.deliveryAddress,
		&row.createdAt,
		&row.updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("query order: %w", err)
	}

	return &row, nil
}

func (r *OrderRepository) fetchOrderItems(ctx context.Context, orderID domain.OrderID) ([]domain.OrderItem, error) {
	const itemsQuery = `
		SELECT product_id, quantity, unit_price, total_price
		FROM order_items
		WHERE order_id = $1
		ORDER BY product_id`

	rows, err := r.pool.Query(ctx, itemsQuery, uuid.UUID(orderID))
	if err != nil {
		return nil, fmt.Errorf("query order items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.OrderItem, 0)
	for rows.Next() {
		var (
			productID uuid.UUID
			quantity  int64
			unitPrice int64
			itemTotal int64
		)

		if err := rows.Scan(&productID, &quantity, &unitPrice, &itemTotal); err != nil {
			return nil, fmt.Errorf("scan order item: %w", err)
		}

		item, err := domain.NewOrderItem(domain.ProductID(productID), quantity, unitPrice)
		if err != nil {
			return nil, fmt.Errorf("build order item: %w", err)
		}

		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate order items: %w", err)
	}

	return items, nil
}

func buildOrder(orderID domain.OrderID, row *orderRow, items []domain.OrderItem) (*domain.Order, error) {
	var payment domain.PaymentID
	if row.paymentID != nil {
		payment = domain.PaymentID(*row.paymentID)
	}

	order, err := domain.NewOrder(
		orderID,
		domain.UserID(row.userID),
		domain.OrderStatus(row.status),
		payment,
		row.deliveryAddress,
		row.createdAt,
		row.updatedAt,
		items...,
	)
	if err != nil {
		return nil, fmt.Errorf("build order: %w", err)
	}

	return order, nil
}

func (r *OrderRepository) Save(ctx context.Context, order *domain.Order) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var paymentID *uuid.UUID
	if order.PaymentID() != (domain.PaymentID{}) {
		id := uuid.UUID(order.PaymentID())
		paymentID = &id
	}

	const upsertOrder = `
		INSERT INTO orders (order_id, user_id, status, total_price, payment_id, delivery_address, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (order_id) DO UPDATE SET
			status = EXCLUDED.status,
			total_price = EXCLUDED.total_price,
			payment_id = EXCLUDED.payment_id,
			delivery_address = EXCLUDED.delivery_address,
			updated_at = EXCLUDED.updated_at`

	_, err = tx.Exec(
		ctx,
		upsertOrder,
		uuid.UUID(order.OrderID()),
		uuid.UUID(order.UserID()),
		string(order.Status()),
		order.TotalPrice(),
		paymentID,
		order.DeliveryAddress(),
		order.CreatedAt(),
		order.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("upsert order: %w", err)
	}

	const deleteItems = `DELETE FROM order_items WHERE order_id = $1`
	if _, err := tx.Exec(ctx, deleteItems, uuid.UUID(order.OrderID())); err != nil {
		return fmt.Errorf("delete order items: %w", err)
	}

	const insertItem = `
		INSERT INTO order_items (order_id, product_id, quantity, unit_price, total_price)
		VALUES ($1, $2, $3, $4, $5)`

	for _, item := range order.Items() {
		_, err := tx.Exec(
			ctx,
			insertItem,
			uuid.UUID(order.OrderID()),
			uuid.UUID(item.ProductID()),
			item.Quantity(),
			item.UnitPrice(),
			item.TotalPrice(),
		)
		if err != nil {
			return fmt.Errorf("insert order item: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (r *OrderRepository) Delete(ctx context.Context, orderID domain.OrderID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const deleteItems = `DELETE FROM order_items WHERE order_id = $1`
	if _, err := tx.Exec(ctx, deleteItems, uuid.UUID(orderID)); err != nil {
		return fmt.Errorf("delete order items: %w", err)
	}

	const deleteOrder = `DELETE FROM orders WHERE order_id = $1`
	if _, err := tx.Exec(ctx, deleteOrder, uuid.UUID(orderID)); err != nil {
		return fmt.Errorf("delete order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (r *OrderRepository) Ping(ctx context.Context) error {
	if err := r.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	return nil
}
