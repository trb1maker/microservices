package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreSaveAndLoad(t *testing.T) {
	store := NewStore("test-session-secret-minimum-32-characters", time.Hour)
	rec := httptest.NewRecorder()
	require.NoError(t, store.Save(rec, Data{
		AccessToken: "token-1",
		UserEmail:   "demo@example.com",
		UserID:      "11111111-1111-4111-8111-111111111111",
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range rec.Result().Cookies() {
		req.AddCookie(cookie)
	}

	data, err := store.Load(req)
	require.NoError(t, err)
	require.Equal(t, "token-1", data.AccessToken)
	require.Equal(t, "demo@example.com", data.UserEmail)
}

func TestStoreClear(t *testing.T) {
	store := NewStore("test-session-secret-minimum-32-characters", time.Hour)
	rec := httptest.NewRecorder()
	require.NoError(t, store.Save(rec, Data{AccessToken: "token-1", UserID: "u1"}))
	store.Clear(rec)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := store.Load(req)
	require.Error(t, err)
}
