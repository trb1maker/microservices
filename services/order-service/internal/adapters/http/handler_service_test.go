package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cartmemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/cart_repository/memory"
	ordermemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/order_repository/memory"
	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgmiddleware "github.com/trb1maker/microservices/pkg/middleware"
)

func TestGetOrder_serviceIdentity(t *testing.T) {
	t.Parallel()

	userID := domain.UserID(uuid.New())
	orderID := domain.OrderID(uuid.New())
	productID := domain.ProductID(uuid.New())

	item, err := domain.NewOrderItem(productID, 1, 100)
	require.NoError(t, err)

	order, err := domain.NewOrder(
		orderID,
		userID,
		domain.OrderStatusPending,
		domain.PaymentID(uuid.Nil),
		"Moscow",
		time.Now().UTC(),
		time.Now().UTC(),
		*item,
	)
	require.NoError(t, err)

	cartRepo := cartmemory.NewCartRepository()
	orderRepo := ordermemory.NewOrderRepository()
	require.NoError(t, orderRepo.Save(t.Context(), order))

	handler := NewHandler(
		app.NewCartService(cartRepo),
		app.NewOrderService(cartRepo, orderRepo, app.NewNoopEventPublisher(), app.NewNoopOrderMetrics()),
		nil,
	)

	req := httptest.NewRequestWithContext(
		pkgmiddleware.ContextWithServiceName(t.Context(), "payment-service"),
		http.MethodGet,
		"/orders/"+uuid.UUID(orderID).String(),
		nil,
	)
	req.SetPathValue("id", uuid.UUID(orderID).String())

	rec := httptest.NewRecorder()
	handler.GetOrder(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body orderResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	assert.Equal(t, uuid.UUID(orderID).String(), body.OrderID)
	assert.Equal(t, "Moscow", body.DeliveryAddress)
}
