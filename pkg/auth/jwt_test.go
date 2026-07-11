package auth_test

import (
	"testing"
	"time"

	"github.com/trb1maker/microservices/pkg/auth"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-secret-minimum-32-characters"

func TestIssueAndValidateToken(t *testing.T) {
	t.Parallel()

	userID := uuid.New()

	token, err := auth.IssueToken(testJWTSecret, userID, time.Hour)
	require.NoError(t, err)

	got, err := auth.ValidateToken(testJWTSecret, token)
	require.NoError(t, err)
	assert.Equal(t, userID, got)
}

func TestValidateToken_wrongSecret(t *testing.T) {
	t.Parallel()

	userID := uuid.New()

	token, err := auth.IssueToken(testJWTSecret, userID, time.Hour)
	require.NoError(t, err)

	_, err = auth.ValidateToken("another-secret-minimum-32-characters", token)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestValidateToken_expired(t *testing.T) {
	t.Parallel()

	userID := uuid.New()

	token, err := auth.IssueToken(testJWTSecret, userID, -time.Minute)
	require.NoError(t, err)

	_, err = auth.ValidateToken(testJWTSecret, token)
	assert.ErrorIs(t, err, auth.ErrExpiredToken)
}

func TestHashAndCheckPassword(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("demo123")
	require.NoError(t, err)
	assert.True(t, auth.CheckPassword(hash, "demo123"))
	assert.False(t, auth.CheckPassword(hash, "wrong"))
}
