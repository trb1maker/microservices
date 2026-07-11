package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	"github.com/google/uuid"
	"github.com/trb1maker/microservices/pkg/auth"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrAuthUnavailable    = errors.New("authentication unavailable")
)

type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
}

type AuthService struct {
	users  UserRepository
	secret string
	ttl    time.Duration
}

func NewAuthService(users UserRepository, secret string, ttl time.Duration) *AuthService {
	return &AuthService{users: users, secret: secret, ttl: ttl}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, time.Duration, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" || password == "" {
		return "", 0, ErrInvalidCredentials
	}

	user, err := s.users.FindByEmail(ctx, normalized)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %w", ErrAuthUnavailable, err)
	}

	if user == nil || !auth.CheckPassword(user.PasswordHash(), password) {
		return "", 0, ErrInvalidCredentials
	}

	token, err := auth.IssueToken(s.secret, uuid.UUID(user.ID()), s.ttl)
	if err != nil {
		return "", 0, fmt.Errorf("issue token: %w", err)
	}

	return token, s.ttl, nil
}
