package http

import (
	"net/http"
	"time"

	"github.com/trb1maker/microservices/pkg/middleware"
)

const readHeaderTimeout = 5 * time.Second

func NewServer(
	addr string,
	handler *Handler,
	httpMetrics HTTPMetrics,
	serviceName string,
	metricsPath string,
) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handler.Health)
	mux.HandleFunc("GET /ready", handler.Ready)
	mux.HandleFunc("POST /cart/items", handler.AddCartItem)
	mux.HandleFunc("GET /cart", handler.GetCart)
	mux.HandleFunc("DELETE /cart/items/{productID}", handler.RemoveCartItem)
	mux.HandleFunc("POST /orders", handler.Checkout)
	mux.HandleFunc("GET /orders/{id}", handler.GetOrder)
	mux.HandleFunc("DELETE /orders/{id}", handler.CancelOrder)
	mux.HandleFunc("GET /debug/error", handler.DebugError)

	httpHandler := http.Handler(mux)
	if httpMetrics != nil || serviceName != "" {
		if metricsPath == "" {
			metricsPath = "/metrics"
		}

		if httpMetrics != nil {
			mux.Handle("GET "+metricsPath, httpMetrics.Handler())
		}

		httpHandler = middleware.Chain(mux, serviceName, httpMetrics, nil)
	}

	return &http.Server{
		Addr:              addr,
		Handler:           httpHandler,
		ReadHeaderTimeout: readHeaderTimeout,
	}
}
