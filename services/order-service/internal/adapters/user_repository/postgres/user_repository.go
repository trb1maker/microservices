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

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	const query = `
		SELECT id, email, password, created_at
		FROM users
		WHERE email = $1`

	var (
		id        uuid.UUID
		stored    string
		password  string
		createdAt time.Time
	)

	err := r.pool.QueryRow(ctx, query, email).Scan(&id, &stored, &password, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}

	return domain.NewUser(domain.UserID(id), stored, password, createdAt), nil
}
