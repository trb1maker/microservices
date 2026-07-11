package http

import (
	"net/http"
	"time"
)

const readHeaderTimeout = 5 * time.Second

func NewServer(addr string, handler *Handler) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handler.Health)
	mux.HandleFunc("GET /ready", handler.Ready)
	mux.HandleFunc("POST /cart/items", handler.AddCartItem)
	mux.HandleFunc("GET /cart", handler.GetCart)
	mux.HandleFunc("DELETE /cart/items/{productID}", handler.RemoveCartItem)
	mux.HandleFunc("POST /orders", handler.Checkout)
	mux.HandleFunc("GET /orders/{id}", handler.GetOrder)
	mux.HandleFunc("DELETE /orders/{id}", handler.CancelOrder)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}
}
