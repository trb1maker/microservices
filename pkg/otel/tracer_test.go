package otel_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trb1maker/microservices/pkg/metrics"
	"github.com/trb1maker/microservices/pkg/middleware"
	pkgotel "github.com/trb1maker/microservices/pkg/otel"
)

func TestInit_disabledReturnsNoopShutdown(t *testing.T) {
	t.Parallel()

	shutdown, err := pkgotel.Init(context.Background(), "test-service", "http://localhost:4318", true)
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
}

func TestInit_unreachableEndpointStillInitializes(t *testing.T) {
	t.Parallel()

	shutdown, err := pkgotel.Init(context.Background(), "test-service", "http://127.0.0.1:1", false)
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
}

func TestChain_servesBusinessRequestWithTracingDisabled(t *testing.T) {
	t.Parallel()

	shutdown, err := pkgotel.Init(context.Background(), "test-service", "http://127.0.0.1:1", true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	m := metrics.New("/metrics")
	handler := middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "test-service", m, nil, "/metrics")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	req.Pattern = "GET /health"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
