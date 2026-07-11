package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if payload == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status, message := mapError(err)
	writeJSON(w, status, errorResponse{Error: message})
}

func mapError(err error) (int, string) {
	switch {
	case errors.Is(err, app.ErrInvalidUserID),
		errors.Is(err, app.ErrInvalidProductID),
		errors.Is(err, app.ErrInvalidOrderID),
		errors.Is(err, app.ErrDeliveryAddressRequired),
		errors.Is(err, domain.ErrNotValidOrderItem):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, domain.ErrItemNotFound),
		errors.Is(err, app.ErrOrderNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, domain.ErrEmptyCart),
		errors.Is(err, domain.ErrOrderCancellationForbidden):
		return http.StatusBadRequest, err.Error()
	default:
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			return http.StatusBadRequest, msgInvalidRequestBody
		}

		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return http.StatusBadRequest, msgInvalidRequestBody
		}

		return http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

type healthResponse struct {
	Status string `json:"status"`
}

type readyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

type addCartItemRequest struct {
	ProductID string `json:"product_id"`
	Quantity  int64  `json:"quantity"`
	UnitPrice int64  `json:"unit_price"`
}

type checkoutRequest struct {
	DeliveryAddress string `json:"delivery_address"`
}

type cartItemResponse struct {
	ProductID  string `json:"product_id"`
	Quantity   int64  `json:"quantity"`
	UnitPrice  int64  `json:"unit_price"`
	TotalPrice int64  `json:"total_price"`
}

type cartResponse struct {
	UserID     string             `json:"user_id"`
	Items      []cartItemResponse `json:"items"`
	TotalPrice int64              `json:"total_price"`
	UpdatedAt  string             `json:"updated_at"`
}

type orderItemResponse struct {
	ProductID  string `json:"product_id"`
	Quantity   int64  `json:"quantity"`
	UnitPrice  int64  `json:"unit_price"`
	TotalPrice int64  `json:"total_price"`
}

type orderResponse struct {
	OrderID         string              `json:"order_id"`
	UserID          string              `json:"user_id"`
	Status          string              `json:"status"`
	TotalPrice      int64               `json:"total_price"`
	DeliveryAddress string              `json:"delivery_address"`
	Items           []orderItemResponse `json:"items"`
	CreatedAt       string              `json:"created_at"`
	UpdatedAt       string              `json:"updated_at"`
}

func toCartResponse(cart *domain.Cart) cartResponse {
	items := make([]cartItemResponse, 0, len(cart.Items()))
	for _, item := range cart.Items() {
		items = append(items, cartItemResponse{
			ProductID:  uuidToString(item.ProductID()),
			Quantity:   item.Quantity(),
			UnitPrice:  item.UnitPrice(),
			TotalPrice: item.TotalPrice(),
		})
	}

	return cartResponse{
		UserID:     uuidToString(cart.UserID()),
		Items:      items,
		TotalPrice: cart.TotalPrice(),
		UpdatedAt:  cart.UpdatedAt().UTC().Format(timeRFC3339),
	}
}

func toOrderResponse(order *domain.Order) orderResponse {
	items := make([]orderItemResponse, 0, len(order.Items()))
	for _, item := range order.Items() {
		items = append(items, orderItemResponse{
			ProductID:  uuidToString(item.ProductID()),
			Quantity:   item.Quantity(),
			UnitPrice:  item.UnitPrice(),
			TotalPrice: item.TotalPrice(),
		})
	}

	return orderResponse{
		OrderID:         uuidToString(order.OrderID()),
		UserID:          uuidToString(order.UserID()),
		Status:          string(order.Status()),
		TotalPrice:      order.TotalPrice(),
		DeliveryAddress: order.DeliveryAddress(),
		Items:           items,
		CreatedAt:       order.CreatedAt().UTC().Format(timeRFC3339),
		UpdatedAt:       order.UpdatedAt().UTC().Format(timeRFC3339),
	}
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

const msgInvalidRequestBody = "invalid request body"
