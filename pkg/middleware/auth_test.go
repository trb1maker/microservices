package middleware_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/trb1maker/microservices/pkg/auth"
	"github.com/trb1maker/microservices/pkg/middleware"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-secret-minimum-32-characters"

func TestJWTAuth_requiresToken(t *testing.T) {
	t.Parallel()

	handler := middleware.JWTAuth(testJWTSecret, nil, nil, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptestRequest(t, http.MethodGet, "/cart", "")
	rec := httptestRecorder(handler, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code())
}

func TestJWTAuth_validToken(t *testing.T) {
	t.Parallel()

	userID := uuid.New()

	token, err := auth.IssueToken(testJWTSecret, userID, time.Hour)
	require.NoError(t, err)

	var got uuid.UUID

	handler := middleware.JWTAuth(testJWTSecret, nil, nil, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := middleware.UserIDFromContext(r.Context())
			assert.True(t, ok)
			got = id
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptestRequest(t, http.MethodGet, "/cart", "Bearer "+token)
	rec := httptestRecorder(handler, req)

	require.Equal(t, http.StatusOK, rec.Code())
	assert.Equal(t, userID, got)
}

func TestJWTAuth_skipPath(t *testing.T) {
	t.Parallel()

	handler := middleware.JWTAuth(testJWTSecret, func(r *http.Request) bool {
		return r.URL.Path == "/health"
	}, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptestRequest(t, http.MethodGet, "/health", "")
	rec := httptestRecorder(handler, req)

	assert.Equal(t, http.StatusOK, rec.Code())
}

func httptestRequest(t *testing.T, method, path, authHeader string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), method, path, http.NoBody)
	require.NoError(t, err)

	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	return req
}

func httptestRecorder(handler http.Handler, req *http.Request) *responseRecorder {
	rec := &responseRecorder{header: make(http.Header)}
	handler.ServeHTTP(rec, req)
	return rec
}

type responseRecorder struct {
	header http.Header
	code   int
	body   []byte
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return len(b), nil
}

func (r *responseRecorder) Code() int {
	if r.code == 0 {
		return http.StatusOK
	}

	return r.code
}
