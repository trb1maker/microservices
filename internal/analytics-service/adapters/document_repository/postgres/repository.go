package postgres

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
)

type DocumentRepository struct {
	pool *pgxpool.Pool
}

func NewDocumentRepository(pool *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{pool: pool}
}

func (r *DocumentRepository) Upsert(ctx context.Context, doc app.ReceiptDocument) error {
	orderID, err := uuid.Parse(doc.OrderID)
	if err != nil {
		return fmt.Errorf("parse order_id: %w", err)
	}
	userID, err := uuid.Parse(doc.UserID)
	if err != nil {
		return fmt.Errorf("parse user_id: %w", err)
	}

	const query = `
		INSERT INTO receipt_documents (
			order_id, user_id, total_amount, status, finalized_at, delivery_address, search_text
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (order_id) DO UPDATE SET
			total_amount = EXCLUDED.total_amount,
			status = EXCLUDED.status,
			finalized_at = EXCLUDED.finalized_at,
			delivery_address = EXCLUDED.delivery_address,
			search_text = EXCLUDED.search_text`

	_, err = r.pool.Exec(ctx, query,
		orderID,
		userID,
		doc.TotalAmount,
		doc.Status,
		doc.FinalizedAt,
		doc.DeliveryAddress,
		doc.SearchText,
	)
	if err != nil {
		return fmt.Errorf("upsert receipt document: %w", err)
	}
	return nil
}

func (r *DocumentRepository) GetByOrderID(ctx context.Context, orderID string) (*app.ReceiptDocument, error) {
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		return nil, fmt.Errorf("parse order_id: %w", err)
	}

	const query = `
		SELECT order_id, user_id, total_amount, status, finalized_at, delivery_address, search_text
		FROM receipt_documents
		WHERE order_id = $1`

	var (
		orderRow uuid.UUID
		userUUID uuid.UUID
		doc      app.ReceiptDocument
	)
	err = r.pool.QueryRow(ctx, query, orderUUID).Scan(
		&orderRow,
		&userUUID,
		&doc.TotalAmount,
		&doc.Status,
		&doc.FinalizedAt,
		&doc.DeliveryAddress,
		&doc.SearchText,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query receipt document: %w", err)
	}
	doc.OrderID = orderRow.String()
	doc.UserID = userUUID.String()
	return &doc, nil
}

func (r *DocumentRepository) Search(ctx context.Context, userID, query string, limit int) ([]app.SearchResult, error) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}

	const sql = `
		SELECT order_id, user_id, total_amount, status, finalized_at, delivery_address
		FROM receipt_documents
		WHERE user_id = $1
		  AND tsv @@ plainto_tsquery('simple', $2)
		ORDER BY finalized_at DESC
		LIMIT $3`

	rows, err := r.pool.Query(ctx, sql, userUUID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search receipt documents: %w", err)
	}
	defer rows.Close()

	results := make([]app.SearchResult, 0)
	for rows.Next() {
		var (
			orderUUID uuid.UUID
			userUUID  uuid.UUID
			item      app.SearchResult
		)
		if err := rows.Scan(
			&orderUUID,
			&userUUID,
			&item.TotalAmount,
			&item.Status,
			&item.FinalizedAt,
			&item.DeliveryAddress,
		); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		item.OrderID = orderUUID.String()
		item.UserID = userUUID.String()
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}
	return results, nil
}

var _ app.DocumentRepository = (*DocumentRepository)(nil)
