package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trb1maker/microservices/scripts/ui/internal/config"
)

func TestHandleSelectUser(t *testing.T) {
	orderAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/auth/login", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "jwt-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	t.Cleanup(orderAPI.Close)

	cfg := &config.Config{
		HTTPAddr:         ":8081",
		OrderHTTPBaseURL: orderAPI.URL,
		OrderGRPCAddr:    "localhost:50052",
		TLSSkipVerify:    true,
		SessionSecret:    "test-session-secret-minimum-32-characters",
	}
	server, err := NewServer(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	req := httptest.NewRequest(http.MethodPost, "/session/user", nil)
	req.Form = map[string][]string{"email": {"demo@example.com"}}
	rec := httptest.NewRecorder()
	server.handleSelectUser(rec, req)

	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.NotEmpty(t, rec.Result().Cookies())

	follow := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range rec.Result().Cookies() {
		follow.AddCookie(cookie)
	}
	data, err := server.sessions.Load(follow)
	require.NoError(t, err)
	require.Equal(t, "jwt-token", data.AccessToken)
}
