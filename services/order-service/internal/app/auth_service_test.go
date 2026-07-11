package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/pkg/auth"
)

var errRepoUnavailable = errors.New("repository unavailable")

type stubUserRepo struct {
	user *domain.User
	err  error
}

func (s stubUserRepo) FindByEmail(_ context.Context, _ string) (*domain.User, error) {
	return s.user, s.err
}

func TestAuthService_Login_success(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("demo123")
	require.NoError(t, err)

	userID := domain.UserID(uuid.New())
	repo := stubUserRepo{user: domain.NewUser(userID, "demo@example.com", hash, time.Now())}
	service := app.NewAuthService(repo, "test-secret-minimum-32-characters", time.Hour)

	token, ttl, err := service.Login(context.Background(), "demo@example.com", "demo123")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, time.Hour, ttl)
}

func TestAuthService_Login_invalidPassword(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("demo123")
	require.NoError(t, err)

	userID := domain.UserID(uuid.New())
	repo := stubUserRepo{user: domain.NewUser(userID, "demo@example.com", hash, time.Now())}
	service := app.NewAuthService(repo, "test-secret-minimum-32-characters", time.Hour)

	_, _, err = service.Login(context.Background(), "demo@example.com", "wrong")
	assert.ErrorIs(t, err, app.ErrInvalidCredentials)
}

func TestAuthService_Login_repositoryError(t *testing.T) {
	t.Parallel()

	repo := stubUserRepo{err: errRepoUnavailable}
	service := app.NewAuthService(repo, "test-secret-minimum-32-characters", time.Hour)

	_, _, err := service.Login(context.Background(), "demo@example.com", "demo123")
	assert.ErrorIs(t, err, app.ErrAuthUnavailable)
}

func TestAuthService_Login_unknownUser(t *testing.T) {
	t.Parallel()

	repo := stubUserRepo{user: nil}
	service := app.NewAuthService(repo, "test-secret-minimum-32-characters", time.Hour)

	_, _, err := service.Login(context.Background(), "missing@example.com", "demo123")
	assert.ErrorIs(t, err, app.ErrInvalidCredentials)
}
