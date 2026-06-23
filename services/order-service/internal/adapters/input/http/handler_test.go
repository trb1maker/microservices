package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"order-service/internal/adapters/output/memory"
	"order-service/internal/app"
	"order-service/internal/domain"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpadapter "order-service/internal/adapters/input/http"
)

func newRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)

	return req
}

func doRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	cartRepo := memory.NewCartRepository()
	orderRepo := memory.NewOrderRepository()
	cartService := app.NewCartService(cartRepo)
	orderService := app.NewOrderService(cartRepo, orderRepo)
	handler := httpadapter.NewHandler(cartService, orderService)

	return httptest.NewServer(httpadapter.NewServer(handler).Handler)
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

	userID := uuid.New().String()
	productID := uuid.New().String()

	req := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+productID+`","quantity":2,"unit_price":100}`,
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", userID)

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	getReq := newRequest(t, http.MethodGet, server.URL+"/cart", "")
	getReq.Header.Set("X-User-ID", userID)

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

func TestAddCartItem_requiresUserID(t *testing.T) {
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAddCartItem_invalidJSON(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	req := newRequest(t, http.MethodPost, server.URL+"/cart/items", "{")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", uuid.New().String())

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCheckout_createsOrder(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	t.Cleanup(server.Close)

	userID := uuid.New().String()
	productID := uuid.New().String()

	addReq := newRequest(
		t,
		http.MethodPost,
		server.URL+"/cart/items",
		`{"product_id":"`+productID+`","quantity":1,"unit_price":100}`,
	)
	addReq.Header.Set("Content-Type", "application/json")
	addReq.Header.Set("X-User-ID", userID)

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
	checkoutReq.Header.Set("X-User-ID", userID)

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
	getCartReq.Header.Set("X-User-ID", userID)

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
	req.Header.Set("X-User-ID", uuid.New().String())

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
	req.Header.Set("X-User-ID", uuid.New().String())

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCancelOrder_confirmedForbidden(t *testing.T) {
	t.Parallel()

	cartRepo := memory.NewCartRepository()
	orderRepo := memory.NewOrderRepository()
	orderService := app.NewOrderService(cartRepo, orderRepo)

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
		order.CreatedAt(),
		order.UpdatedAt(),
		order.Items()...,
	)
	require.NoError(t, err)
	require.NoError(t, orderRepo.Save(t.Context(), confirmed))

	handler := httpadapter.NewHandler(app.NewCartService(cartRepo), orderService)
	testServer := httptest.NewServer(httpadapter.NewServer(handler).Handler)
	t.Cleanup(testServer.Close)

	req := newRequest(
		t,
		http.MethodDelete,
		testServer.URL+"/orders/"+uuid.UUID(order.OrderID()).String(),
		"",
	)

	resp := doRequest(t, req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
