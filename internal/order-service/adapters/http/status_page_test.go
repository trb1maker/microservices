package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cartmemory "github.com/trb1maker/microservices/internal/order-service/adapters/cart_repository/memory"
	httpadapter "github.com/trb1maker/microservices/internal/order-service/adapters/http"
	ordermemory "github.com/trb1maker/microservices/internal/order-service/adapters/order_repository/memory"
	"github.com/trb1maker/microservices/internal/order-service/app"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCheckPostgres = "postgres"

var errPaymentUnavailable = errors.New("payment unavailable")

type stubReadinessChecker struct {
	ready  bool
	checks map[string]string
}

func (s stubReadinessChecker) Check(context.Context) (bool, map[string]string) {
	return s.ready, s.checks
}

type stubPaymentHealth struct {
	err error
}

func (s stubPaymentHealth) CheckHealth(context.Context) error {
	return s.err
}

type slowRemoteChecker struct {
	delay time.Duration
}

func (s slowRemoteChecker) CheckPayment(ctx context.Context) httpadapter.StatusItem {
	select {
	case <-ctx.Done():
		return httpadapter.StatusItem{
			Name:        "Payment Service",
			Status:      "down",
			StatusClass: "down",
			Detail:      ctx.Err().Error(),
			Endpoint:    "payment-service:50051",
		}
	case <-time.After(s.delay):
		return httpadapter.StatusItem{
			Name:        "Payment Service",
			Status:      "ok",
			StatusClass: "ok",
			Detail:      "grpc health SERVING",
			Endpoint:    "payment-service:50051",
		}
	}
}

func (s slowRemoteChecker) CheckStore(context.Context) httpadapter.StatusItem {
	return httpadapter.StatusItem{
		Name:        "Store Service",
		Status:      "ok",
		StatusClass: "ok",
		Detail:      "ready",
		Endpoint:    "http://store-service:9092/ready",
	}
}

func newHealthTestServer(t *testing.T, readiness httpadapter.ReadinessChecker, remote httpadapter.RemoteHealthChecker) *httptest.Server {
	t.Helper()

	dashboard, err := httpadapter.NewStatusDashboard(readiness, remote)
	require.NoError(t, err)

	cartRepo := cartmemory.NewCartRepository()
	orderRepo := ordermemory.NewOrderRepository()
	handler := httpadapter.NewHandler(
		app.NewCartService(cartRepo),
		app.NewOrderService(cartRepo, orderRepo, app.NewNoopEventPublisher(), app.NewNoopOrderMetrics()),
		readiness,
		dashboard,
	)

	return httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{
		Addr: testServerAddr,
		Auth: &httpadapter.AuthConfig{JWTSecret: testJWTSecret},
	}, handler, nil, nil).Handler)
}

func TestHealth_returnsJSONByDefault(t *testing.T) {
	t.Parallel()

	server := newHealthTestServer(t, stubReadinessChecker{ready: true, checks: map[string]string{testCheckPostgres: "ok"}}, nil)
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodGet, server.URL+"/health", "")
	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body.Status)
}

func TestHealth_returnsHTMLDashboard(t *testing.T) {
	t.Parallel()

	server := newHealthTestServer(t, stubReadinessChecker{
		ready:  true,
		checks: map[string]string{testCheckPostgres: "ok", "nats": "ok"},
	}, httpadapter.NewHTTPRemoteChecker("", &stubPaymentHealth{}))
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodGet, server.URL+"/health", "")
	req.Header.Set("Accept", "text/html")
	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	html := string(body)
	assert.Contains(t, html, "Order Service Health")
	assert.Contains(t, html, "PostgreSQL (orders)")
	assert.Contains(t, html, "Payment Service")
}

func TestHealth_escapesHTMLInDetails(t *testing.T) {
	t.Parallel()

	server := newHealthTestServer(t, stubReadinessChecker{
		ready:  false,
		checks: map[string]string{testCheckPostgres: "<script>alert(1)</script>"},
	}, nil)
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodGet, server.URL+"/health", "")
	req.Header.Set("Accept", "text/html")
	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	html := string(body)
	assert.NotContains(t, html, "<script>alert(1)</script>")
	assert.Contains(t, html, "&lt;script&gt;alert(1)&lt;/script&gt;")
}

func TestHealth_degradedWhenDependenciesFail(t *testing.T) {
	t.Parallel()

	server := newHealthTestServer(t, stubReadinessChecker{
		ready:  false,
		checks: map[string]string{testCheckPostgres: "connection refused"},
	}, httpadapter.NewHTTPRemoteChecker("", &stubPaymentHealth{err: errPaymentUnavailable}))
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodGet, server.URL+"/health", "")
	req.Header.Set("Accept", "text/html")
	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	html := string(body)
	assert.Contains(t, html, "degraded")
	assert.Contains(t, html, "connection refused")
	assert.Contains(t, html, "payment unavailable")
}

func TestStatusDashboard_respectsContextTimeout(t *testing.T) {
	t.Parallel()

	dashboard, err := httpadapter.NewStatusDashboard(
		stubReadinessChecker{ready: true, checks: map[string]string{"nats": "ok"}},
		slowRemoteChecker{delay: 5 * time.Second},
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	data := dashboard.Build(ctx)
	require.NotEmpty(t, data.Items)

	var paymentItem httpadapter.StatusItem
	for _, item := range data.Items {
		if strings.Contains(item.Name, "Payment") {
			paymentItem = item
			break
		}
	}
	assert.Equal(t, "down", paymentItem.Status)
}
