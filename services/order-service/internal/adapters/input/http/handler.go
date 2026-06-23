package http

import (
	"encoding/json"
	"net/http"
	"order-service/internal/app"
	"order-service/internal/domain"
	"time"
)

type Handler struct {
	carts  *app.CartService
	orders *app.OrderService
}

func NewHandler(carts *app.CartService, orders *app.OrderService) *Handler {
	return &Handler{carts: carts, orders: orders}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func (h *Handler) AddCartItem(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserID(r)
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
	userID, err := parseUserID(r)
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
	userID, err := parseUserID(r)
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
	userID, err := parseUserID(r)
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
	orderID, err := parseOrderID(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	order, err := h.orders.GetOrder(r.Context(), orderID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toOrderResponse(order))
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	orderID, err := parseOrderID(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	order, err := h.orders.CancelOrder(r.Context(), orderID, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toOrderResponse(order))
}
