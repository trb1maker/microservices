package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/pkg/health"
)

var errDatabaseDown = errors.New("connection refused")

func TestLivenessHandler(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	health.LivenessHandler()(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	assert.Equal(t, "ok", body.Status)
}

func TestReadinessHandler_ready(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	health.ReadinessHandler(health.NewChecker(map[string]health.CheckFunc{
		"postgres": func(context.Context) error { return nil },
	}))(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	assert.Equal(t, "ready", body.Status)
	assert.Equal(t, "ok", body.Checks["postgres"])
}

func TestReadinessHandler_notReady(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	health.ReadinessHandler(health.NewChecker(map[string]health.CheckFunc{
		"postgres": func(context.Context) error { return errDatabaseDown },
	}))(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	assert.Equal(t, "not_ready", body.Status)
	assert.Equal(t, "connection refused", body.Checks["postgres"])
}
