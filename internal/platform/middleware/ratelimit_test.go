//go:build integration

package middleware_test

import (
	"context"
	"net/http"
	"testing"
	"time"
	"uuid"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/trb1maker/microservices/internal/platform/auth"
	"github.com/trb1maker/microservices/internal/platform/middleware"
)

func TestRateLimit_blocksAfterLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:8.8-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	opts, err := goredis.ParseURL(connStr)
	require.NoError(t, err)

	client := goredis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })

	userID := uuid.NewV7()
	token, err := auth.IssueToken(testJWTSecret, userID, time.Hour)
	require.NoError(t, err)

	handler := middleware.JWTAuth(testJWTSecret, nil, nil, nil)(
		middleware.RateLimit(middleware.RateLimitConfig{
			Client: client,
			Limit:  2,
			Window: time.Minute,
		})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})),
	)

	for range 2 {
		req := httptestRequest(t, http.MethodGet, "/cart", "Bearer "+token)
		rec := httptestRecorder(handler, req)
		require.Equal(t, http.StatusOK, rec.Code())
	}

	req := httptestRequest(t, http.MethodGet, "/cart", "Bearer "+token)
	rec := httptestRecorder(handler, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code())
	assert.NotEmpty(t, rec.header.Get("Retry-After"))
}

func TestRateLimit_usesIPForLogin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:8.8-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	opts, err := goredis.ParseURL(connStr)
	require.NoError(t, err)

	client := goredis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })

	handler := middleware.RateLimit(middleware.RateLimitConfig{
		Client: client,
		Limit:  1,
		Window: time.Minute,
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptestRequest(t, http.MethodPost, "/auth/login", "")
	req.RemoteAddr = "203.0.113.10:12345"
	rec := httptestRecorder(handler, req)
	require.Equal(t, http.StatusOK, rec.Code())

	req = httptestRequest(t, http.MethodPost, "/auth/login", "")
	req.RemoteAddr = "203.0.113.10:12345"
	rec = httptestRecorder(handler, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code())
}
