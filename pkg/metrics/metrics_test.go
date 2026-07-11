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

func TestInstrument_IncrementsCounter(t *testing.T) {
	t.Parallel()

	m := New()
	handler := m.Instrument(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/orders", nil)
	req.Pattern = "POST /orders"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.InEpsilon(t, float64(1), testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("POST", "POST /orders", "201")), 0.001)
}
