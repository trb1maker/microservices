package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trb1maker/microservices/services/payment-service/internal/domain"
)

// TransactionRepository implements app.TransactionRepository using PostgreSQL.
type TransactionRepository struct {
	pool *pgxpool.Pool
}

// NewTransactionRepository creates a new TransactionRepository.
func NewTransactionRepository(pool *pgxpool.Pool) *TransactionRepository {
	return &TransactionRepository{pool: pool}
}

// Create inserts a new transaction.
func (r *TransactionRepository) Create(ctx context.Context, tx *domain.Transaction) error {
	query := `
		INSERT INTO transactions (id, order_id, user_id, type, amount, status, original_transaction_id, failure_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	var origTxID *string
	if tx.OriginalTransactionID != nil {
		s := string(*tx.OriginalTransactionID)
		origTxID = &s
	}

	_, err := r.pool.Exec(ctx, query,
		string(tx.ID),
		string(tx.OrderID),
		string(tx.UserID),
		string(tx.Type),
		tx.Amount,
		string(tx.Status),
		origTxID,
		tx.FailureReason,
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	return nil
}

// Get retrieves a transaction by ID.
func (r *TransactionRepository) Get(ctx context.Context, id domain.TransactionID) (*domain.Transaction, error) {
	query := `
		SELECT id, order_id, user_id, type, amount, status, original_transaction_id, failure_reason
		FROM transactions WHERE id = $1`

	var tx domain.Transaction
	var origTxID *string

	err := r.pool.QueryRow(ctx, query, string(id)).Scan(
		(*string)(&tx.ID),
		(*string)(&tx.OrderID),
		(*string)(&tx.UserID),
		(*string)(&tx.Type),
		&tx.Amount,
		(*string)(&tx.Status),
		&origTxID,
		&tx.FailureReason,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTransactionNotFound
		}
		return nil, fmt.Errorf("query transaction: %w", err)
	}

	if origTxID != nil {
		tid := domain.TransactionID(*origTxID)
		tx.OriginalTransactionID = &tid
	}

	return &tx, nil
}

// GetRefundForOriginal returns a refund transaction for the given original charge transaction.
func (r *TransactionRepository) GetRefundForOriginal(ctx context.Context, originalID domain.TransactionID) (*domain.Transaction, error) {
	query := `
		SELECT id, order_id, user_id, type, amount, status, original_transaction_id, failure_reason
		FROM transactions WHERE original_transaction_id = $1 AND type = 'refund'
		LIMIT 1`

	var tx domain.Transaction
	var origTxID *string

	err := r.pool.QueryRow(ctx, query, string(originalID)).Scan(
		(*string)(&tx.ID),
		(*string)(&tx.OrderID),
		(*string)(&tx.UserID),
		(*string)(&tx.Type),
		&tx.Amount,
		(*string)(&tx.Status),
		&origTxID,
		&tx.FailureReason,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTransactionNotFound
		}
		return nil, fmt.Errorf("query refund: %w", err)
	}

	if origTxID != nil {
		tid := domain.TransactionID(*origTxID)
		tx.OriginalTransactionID = &tid
	}

	return &tx, nil
}

// UpdateStatus updates the status of a transaction.
func (r *TransactionRepository) UpdateStatus(ctx context.Context, id domain.TransactionID, status domain.TransactionStatus, failureReason string) error {
	query := `UPDATE transactions SET status = $1, failure_reason = $2 WHERE id = $3`

	_, err := r.pool.Exec(ctx, query, string(status), failureReason, string(id))
	if err != nil {
		return fmt.Errorf("update transaction status: %w", err)
	}

	return nil
}
