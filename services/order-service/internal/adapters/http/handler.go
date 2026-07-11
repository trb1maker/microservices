package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/trb1maker/microservices/services/order-service/internal/domain"
)

type Handler struct {
	carts     CartService
	orders    OrderService
	readiness ReadinessChecker
}

func NewHandler(carts CartService, orders OrderService, readiness ReadinessChecker) *Handler {
	return &Handler{carts: carts, orders: orders, readiness: readiness}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if h.readiness == nil {
		writeJSON(w, http.StatusOK, readyResponse{Status: "ready", Checks: map[string]string{}})
		return
	}

	ready, checks := h.readiness.Check(r.Context())
	if !ready {
		writeJSON(w, http.StatusServiceUnavailable, readyResponse{Status: "not_ready", Checks: checks})
		return
	}

	writeJSON(w, http.StatusOK, readyResponse{Status: "ready", Checks: checks})
}

func (h *Handler) AddCartItem(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req addCartItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: msgInvalidRequestBody})
		return
	}

	productID, err := parseProductID(req.ProductID)
	if err != nil {
		writeError(w, err)
		return
	}

	item, err := domain.NewOrderItem(productID, req.Quantity, req.UnitPrice)
	if err != nil {
		writeError(w, err)
		return
	}

	cart, err := h.carts.AddItem(r.Context(), userID, *item)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toCartResponse(cart))
}

func (h *Handler) GetCart(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	cart, err := h.carts.GetCart(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toCartResponse(cart))
}

func (h *Handler) RemoveCartItem(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	productID, err := parseProductID(r.PathValue("productID"))
	if err != nil {
		writeError(w, err)
		return
	}

	cart, err := h.carts.RemoveItem(r.Context(), userID, productID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toCartResponse(cart))
}

func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: msgInvalidRequestBody})
		return
	}

	order, err := h.orders.Checkout(r.Context(), userID, req.DeliveryAddress, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toOrderResponse(order))
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	caller, err := callerFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	orderID, err := parseOrderID(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	var order *domain.Order
	if caller.isService {
		order, err = h.orders.GetOrderForService(r.Context(), orderID)
	} else {
		order, err = h.orders.GetOrder(r.Context(), caller.userID, orderID)
	}
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toOrderResponse(order))
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	orderID, err := parseOrderID(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	order, err := h.orders.CancelOrder(r.Context(), userID, orderID, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toOrderResponse(order))
}

func (h *Handler) DebugError(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "intentional error for alert demo"})
}
