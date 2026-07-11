package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trb1maker/microservices/pkg/metrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestChain_SkipsMetricsPath(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	called := false

	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), "test-service", m, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestChain_InstrumentsRegularRequest(t *testing.T) {
	t.Parallel()

	m := metrics.New()

	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "test-service", m, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	req.Pattern = "GET /health"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestChain_AccessLogIncludesTraceID(t *testing.T) {
	t.Parallel()

	var logBuf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(slog.Default())
	})

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	m := metrics.New()
	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "test-service", m, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/orders", nil)
	req.Pattern = "POST /orders"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var entry struct {
		Msg     string `json:"msg"`
		TraceID string `json:"trace_id"`
	}
	require.NoError(t, json.Unmarshal(logBuf.Bytes(), &entry))
	assert.Equal(t, "http_request", entry.Msg)
	assert.NotEmpty(t, entry.TraceID)
}
