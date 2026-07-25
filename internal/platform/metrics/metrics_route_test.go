package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstrument_usesUnknownRouteForUnmatchedPaths(t *testing.T) {
	t.Parallel()

	m := New("/metrics")
	handler := m.Instrument(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/orders/11111111-1111-4111-8111-111111111111", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.InEpsilon(t,
		float64(1),
		testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("GET", "unknown", "404")),
		0.001,
	)
}
