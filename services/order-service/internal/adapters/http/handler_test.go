package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cartmemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/cart_repository/memory"
	httpadapter "github.com/trb1maker/microservices/services/order-service/internal/adapters/http"
	ordermemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/order_repository/memory"
	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/pkg/auth"
)

const (
	testJWTSecret  = "test-secret-minimum-32-characters-long"
	testServerAddr = ":8080"
)

func newRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)

	return req
}

func withBearer(t *testing.T, req *http.Request, userID uuid.UUID) {
	t.Helper()

	token, err := auth.IssueToken(testJWTSecret, userID, time.Hour)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
}

func doRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	cartRepo := cartmemory.NewCartRepository()
	orderRepo := ordermemory.NewOrderRepository()
	cartService := app.NewCartService(cartRepo)
	orderService := app.NewOrderService(cartRepo, orderRepo, app.NewNoopEventPublisher(), app.NewNoopOrderMetrics())
	handler := httpadapter.NewHandler(cartService, orderService, nil)

	return httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{
		Addr: testServerAddr,
		Auth: &httpadapter.AuthConfig{JWTSecret: testJWTSecret},
	}, handler, nil, nil).Handler)
}

func TestHealth(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	resp := doRequest(t, newRequest(t, http.MethodGet, server.URL+"/health", ""))
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body.Status)
}

func TestAddCartItem_and_GetCart(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	userID := uuid.New()
	productID := uuid.New().String()

	req := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+productID+`","quantity":2,"unit_price":100}`,
	)
	req.Header.Set("Content-Type", "application/json")
	withBearer(t, req, userID)

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	getReq := newRequest(t, http.MethodGet, server.URL+"/cart", "")
	withBearer(t, getReq, userID)

	getResp := doRequest(t, getReq)
	t.Cleanup(func() { _ = getResp.Body.Close() })
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var cart struct {
		TotalPrice int64 `json:"total_price"`
		Items      []struct {
			Quantity int64 `json:"quantity"`
		} `json:"items"`
	}
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&cart))
	assert.Equal(t, int64(200), cart.TotalPrice)
	require.Len(t, cart.Items, 1)
	assert.Equal(t, int64(2), cart.Items[0].Quantity)
}

func TestAddCartItem_requiresAuthorization(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	req := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+uuid.New().String()+`","quantity":1,"unit_price":100}`,
	)
	req.Header.Set("Content-Type", "application/json")

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAddCartItem_invalidJSON(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodPost, server.URL+"/cart/items", "{")
	req.Header.Set("Content-Type", "application/json")
	withBearer(t, req, uuid.New())

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCheckout_createsOrder(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	userID := uuid.New()
	productID := uuid.New().String()

	addReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+productID+`","quantity":1,"unit_price":100}`,
	)
	addReq.Header.Set("Content-Type", "application/json")
	withBearer(t, addReq, userID)

	addResp := doRequest(t, addReq)
	_ = addResp.Body.Close()
	require.Equal(t, http.StatusCreated, addResp.StatusCode)

	checkoutReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/orders",
		`{"delivery_address":"Moscow, Red Square 1"}`,
	)
	checkoutReq.Header.Set("Content-Type", "application/json")
	withBearer(t, checkoutReq, userID)

	checkoutResp := doRequest(t, checkoutReq)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)

	var order struct {
		Status     string `json:"status"`
		TotalPrice int64  `json:"total_price"`
		OrderID    string `json:"order_id"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))
	assert.Equal(t, string(domain.OrderStatusPending), order.Status)
	assert.Equal(t, int64(100), order.TotalPrice)
	assert.NotEmpty(t, order.OrderID)

	getCartReq := newRequest(t, http.MethodGet, server.URL+"/cart", "")
	withBearer(t, getCartReq, userID)

	getCartResp := doRequest(t, getCartReq)
	t.Cleanup(func() { _ = getCartResp.Body.Close() })

	var cart struct {
		Items []any `json:"items"`
	}
	require.NoError(t, json.NewDecoder(getCartResp.Body).Decode(&cart))
	assert.Empty(t, cart.Items)
}

func TestCheckout_requiresDeliveryAddress(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodPost, server.URL+"/orders", `{"delivery_address":""}`)
	req.Header.Set("Content-Type", "application/json")
	withBearer(t, req, uuid.New())

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCheckout_emptyCart(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodPost, server.URL+"/orders", `{"delivery_address":"Moscow"}`)
	req.Header.Set("Content-Type", "application/json")
	withBearer(t, req, uuid.New())

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCancelOrder_confirmedForbidden(t *testing.T) {
	t.Parallel()

	cartRepo := cartmemory.NewCartRepository()
	orderRepo := ordermemory.NewOrderRepository()
	orderService := app.NewOrderService(cartRepo, orderRepo, app.NewNoopEventPublisher(), app.NewNoopOrderMetrics())

	userID := domain.UserID(uuid.New())
	item, err := domain.NewOrderItem(domain.ProductID(uuid.New()), 1, 100)
	require.NoError(t, err)

	cart, err := domain.NewCart(userID, *item)
	require.NoError(t, err)
	require.NoError(t, cartRepo.Save(t.Context(), cart))

	order, err := orderService.Checkout(t.Context(), userID, "Moscow", cart.UpdatedAt())
	require.NoError(t, err)

	confirmed, err := domain.NewOrder(
		order.OrderID(),
		order.UserID(),
		domain.OrderStatusConfirmed,
		domain.PaymentID(uuid.New()),
		order.DeliveryAddress(),
		order.CreatedAt(),
		order.UpdatedAt(),
		order.Items()...,
	)
	require.NoError(t, err)
	require.NoError(t, orderRepo.Save(t.Context(), confirmed))

	handler := httpadapter.NewHandler(app.NewCartService(cartRepo), orderService, nil)
	testServer := httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{Addr: testServerAddr, Auth: &httpadapter.AuthConfig{JWTSecret: testJWTSecret}}, handler, nil, nil).Handler)
	t.Cleanup(testServer.Close)

	req := newRequest(
		t,
		http.MethodDelete,
		testServer.URL+"/orders/"+uuid.UUID(order.OrderID()).String(),
		"",
	)
	withBearer(t, req, uuid.UUID(userID))

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGetOrder_success(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	userID := uuid.New()
	productID := uuid.New().String()

	addReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+productID+`","quantity":1,"unit_price":100}`,
	)
	addReq.Header.Set("Content-Type", "application/json")
	withBearer(t, addReq, userID)
	_ = doRequest(t, addReq).Body.Close()

	checkoutReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/orders",
		`{"delivery_address":"Moscow, Red Square 1"}`,
	)
	checkoutReq.Header.Set("Content-Type", "application/json")
	withBearer(t, checkoutReq, userID)

	checkoutResp := doRequest(t, checkoutReq)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)

	var checkoutBody struct {
		OrderID         string `json:"order_id"`
		DeliveryAddress string `json:"delivery_address"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&checkoutBody))

	getReq := newRequest(t, http.MethodGet, server.URL+"/orders/"+checkoutBody.OrderID, "")
	withBearer(t, getReq, userID)

	getResp := doRequest(t, getReq)
	t.Cleanup(func() { _ = getResp.Body.Close() })
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var order struct {
		DeliveryAddress string `json:"delivery_address"`
	}
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&order))
	assert.Equal(t, "Moscow, Red Square 1", order.DeliveryAddress)
}

func TestGetOrder_wrongUser(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	ownerID := uuid.New()
	otherID := uuid.New()
	productID := uuid.New().String()

	addReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+productID+`","quantity":1,"unit_price":100}`,
	)
	addReq.Header.Set("Content-Type", "application/json")
	withBearer(t, addReq, ownerID)
	_ = doRequest(t, addReq).Body.Close()

	checkoutReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/orders",
		`{"delivery_address":"Moscow"}`,
	)
	checkoutReq.Header.Set("Content-Type", "application/json")
	withBearer(t, checkoutReq, ownerID)

	checkoutResp := doRequest(t, checkoutReq)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })

	var checkoutBody struct {
		OrderID string `json:"order_id"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&checkoutBody))

	getReq := newRequest(t, http.MethodGet, server.URL+"/orders/"+checkoutBody.OrderID, "")
	withBearer(t, getReq, otherID)

	getResp := doRequest(t, getReq)
	t.Cleanup(func() { _ = getResp.Body.Close() })
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

func TestCancelOrder_success(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	userID := uuid.New()
	productID := uuid.New().String()

	addReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+productID+`","quantity":1,"unit_price":100}`,
	)
	addReq.Header.Set("Content-Type", "application/json")
	withBearer(t, addReq, userID)
	_ = doRequest(t, addReq).Body.Close()

	checkoutReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/orders",
		`{"delivery_address":"Moscow"}`,
	)
	checkoutReq.Header.Set("Content-Type", "application/json")
	withBearer(t, checkoutReq, userID)

	checkoutResp := doRequest(t, checkoutReq)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })

	var checkoutBody struct {
		OrderID string `json:"order_id"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&checkoutBody))

	cancelReq := newRequest(
		t,
		http.MethodDelete,
		server.URL+"/orders/"+checkoutBody.OrderID,
		"",
	)
	withBearer(t, cancelReq, userID)

	cancelResp := doRequest(t, cancelReq)
	t.Cleanup(func() { _ = cancelResp.Body.Close() })
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	var cancelled struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(cancelResp.Body).Decode(&cancelled))
	assert.Equal(t, string(domain.OrderStatusCancelled), cancelled.Status)
}

func TestRemoveCartItem_success(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	userID := uuid.New()
	productID := uuid.New().String()

	addReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+productID+`","quantity":1,"unit_price":100}`,
	)
	addReq.Header.Set("Content-Type", "application/json")
	withBearer(t, addReq, userID)
	_ = doRequest(t, addReq).Body.Close()

	removeReq := newRequest(t, http.MethodDelete, server.URL+"/cart/items/"+productID, "")
	withBearer(t, removeReq, userID)

	removeResp := doRequest(t, removeReq)
	t.Cleanup(func() { _ = removeResp.Body.Close() })
	require.Equal(t, http.StatusOK, removeResp.StatusCode)

	var cart struct {
		Items []any `json:"items"`
	}
	require.NoError(t, json.NewDecoder(removeResp.Body).Decode(&cart))
	assert.Empty(t, cart.Items)
}

type failingReadinessChecker struct{}

func (failingReadinessChecker) Check(context.Context) (bool, map[string]string) {
	return false, map[string]string{"postgres": "connection refused"}
}

func TestReady_notReady(t *testing.T) {
	t.Parallel()

	cartRepo := cartmemory.NewCartRepository()
	orderRepo := ordermemory.NewOrderRepository()
	cartService := app.NewCartService(cartRepo)
	orderService := app.NewOrderService(cartRepo, orderRepo, app.NewNoopEventPublisher(), app.NewNoopOrderMetrics())
	handler := httpadapter.NewHandler(cartService, orderService, failingReadinessChecker{})

	server := httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{Addr: testServerAddr, Auth: &httpadapter.AuthConfig{JWTSecret: testJWTSecret}}, handler, nil, nil).Handler)
	t.Cleanup(server.Close)

	resp := doRequest(t, newRequest(t, http.MethodGet, server.URL+"/ready", ""))
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "not_ready", body.Status)
	assert.Equal(t, "connection refused", body.Checks["postgres"])
}

func TestAddCartItem_invalidToken(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	req := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+uuid.New().String()+`","quantity":1,"unit_price":100}`,
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer invalid-token")

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetOrder_invalidOrderID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodGet, server.URL+"/orders/not-a-uuid", "")
	withBearer(t, req, uuid.New())

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRemoveCartItem_invalidProductID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodDelete, server.URL+"/cart/items/not-a-uuid", "")
	withBearer(t, req, uuid.New())

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

type errorCartService struct{}

var errDatabaseUnavailable = errors.New("database unavailable")

func (errorCartService) AddItem(context.Context, domain.UserID, domain.OrderItem) (*domain.Cart, error) {
	return nil, errDatabaseUnavailable
}

func (errorCartService) GetCart(context.Context, domain.UserID) (*domain.Cart, error) {
	return nil, errDatabaseUnavailable
}

func (errorCartService) RemoveItem(context.Context, domain.UserID, domain.ProductID) (*domain.Cart, error) {
	return nil, errDatabaseUnavailable
}

func TestGetCart_internalError(t *testing.T) {
	t.Parallel()

	orderService := app.NewOrderService(
		cartmemory.NewCartRepository(),
		ordermemory.NewOrderRepository(),
		app.NewNoopEventPublisher(),
		app.NewNoopOrderMetrics(),
	)
	handler := httpadapter.NewHandler(errorCartService{}, orderService, nil)
	server := httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{Addr: testServerAddr, Auth: &httpadapter.AuthConfig{JWTSecret: testJWTSecret}}, handler, nil, nil).Handler)
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodGet, server.URL+"/cart", "")
	withBearer(t, req, uuid.New())

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
