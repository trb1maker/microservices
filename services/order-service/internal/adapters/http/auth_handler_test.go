package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpadapter "github.com/trb1maker/microservices/services/order-service/internal/adapters/http"
	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/pkg/auth"
)

var errRepoUnavailable = errors.New("repository unavailable")

type loginUserRepo struct {
	user *domain.User
	err  error
}

func (r loginUserRepo) FindByEmail(_ context.Context, _ string) (*domain.User, error) {
	return r.user, r.err
}

func newAuthTestServer(t *testing.T, repo app.UserRepository) *httptest.Server {
	t.Helper()

	authService := app.NewAuthService(repo, testJWTSecret, time.Hour)
	handler := httpadapter.NewHandler(nil, nil, nil)

	return httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{
		Addr: testServerAddr,
		Auth: &httpadapter.AuthConfig{JWTSecret: testJWTSecret},
	}, handler, httpadapter.NewAppAuthAdapter(authService), nil).Handler)
}

func TestLogin_success(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("demo123")
	require.NoError(t, err)

	userID := domain.UserID(uuid.New())
	repo := loginUserRepo{user: domain.NewUser(userID, "demo@example.com", hash, time.Now())}
	server := newAuthTestServer(t, repo)
	t.Cleanup(server.Close)

	req := newRequest(
		t,
		http.MethodPost,
		server.URL+"/auth/login",
		`{"email":"demo@example.com","password":"demo123"}`,
	)
	req.Header.Set("Content-Type", "application/json")

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotEmpty(t, body.AccessToken)
	assert.Equal(t, "Bearer", body.TokenType)
	assert.Positive(t, body.ExpiresIn)
}

func TestLogin_invalidPassword(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("demo123")
	require.NoError(t, err)

	userID := domain.UserID(uuid.New())
	repo := loginUserRepo{user: domain.NewUser(userID, "demo@example.com", hash, time.Now())}
	server := newAuthTestServer(t, repo)
	t.Cleanup(server.Close)

	req := newRequest(
		t,
		http.MethodPost,
		server.URL+"/auth/login",
		`{"email":"demo@example.com","password":"wrong"}`,
	)
	req.Header.Set("Content-Type", "application/json")

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestLogin_invalidJSON(t *testing.T) {
	t.Parallel()

	server := newAuthTestServer(t, loginUserRepo{})
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodPost, server.URL+"/auth/login", `{invalid`)
	req.Header.Set("Content-Type", "application/json")

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLogin_authUnavailable(t *testing.T) {
	t.Parallel()

	repo := loginUserRepo{err: errRepoUnavailable}
	server := newAuthTestServer(t, repo)
	t.Cleanup(server.Close)

	req := newRequest(
		t,
		http.MethodPost,
		server.URL+"/auth/login",
		`{"email":"demo@example.com","password":"demo123"}`,
	)
	req.Header.Set("Content-Type", "application/json")

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
