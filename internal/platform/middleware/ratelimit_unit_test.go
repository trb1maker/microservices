//go:build !integration

package middleware_test

import (
	"context"
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/alicebob/miniredis/v2"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trb1maker/microservices/internal/platform/auth"
	"github.com/trb1maker/microservices/internal/platform/middleware"
)

func TestRateLimit_blocksAfterLimit_miniredis(t *testing.T) {
	t.Parallel()

	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	client := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
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
}

func TestRateLimit_disabledWhenLimitZero(t *testing.T) {
	t.Parallel()

	handler := middleware.RateLimit(middleware.RateLimitConfig{
		Limit: 0,
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/cart", http.NoBody)
	require.NoError(t, err)

	rec := httptestRecorder(handler, req)
	assert.Equal(t, http.StatusOK, rec.Code())
}

func TestRateLimit_skipsHealthPaths(t *testing.T) {
	t.Parallel()

	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	client := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	handler := middleware.RateLimit(middleware.RateLimitConfig{
		Client: client,
		Limit:  1,
		Window: time.Minute,
		Skip:   middleware.SkipRateLimitPaths("/metrics"),
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/health", "/ready", "/metrics"} {
		for range 3 {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, path, http.NoBody)
			require.NoError(t, err)

			rec := httptestRecorder(handler, req)
			assert.Equal(t, http.StatusOK, rec.Code(), path)
		}
	}
}
