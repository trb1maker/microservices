package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trb1maker/microservices/services/payment-service/internal/domain"
)

// AccountRepository implements app.AccountRepository using PostgreSQL.
type AccountRepository struct {
	pool *pgxpool.Pool
}

// NewAccountRepository creates a new AccountRepository.
func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

// Get retrieves an account by user ID.
func (r *AccountRepository) Get(ctx context.Context, userID domain.UserID) (*domain.Account, error) {
	query := `SELECT user_id, balance, version FROM accounts WHERE user_id = $1`

	var acc domain.Account
	err := r.pool.QueryRow(ctx, query, string(userID)).Scan(
		(*string)(&acc.UserID),
		&acc.Balance,
		&acc.Version,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("query account: %w", err)
	}

	return &acc, nil
}

// UpdateBalance atomically updates the balance using optimistic locking.
func (r *AccountRepository) UpdateBalance(ctx context.Context, account *domain.Account) error {
	query := `UPDATE accounts SET balance = $1, version = version + 1 WHERE user_id = $2 AND version = $3`

	tag, err := r.pool.Exec(ctx, query, account.Balance, string(account.UserID), account.Version)
	if err != nil {
		return fmt.Errorf("update balance: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrConcurrentModification
	}

	account.Version++

	return nil
}
