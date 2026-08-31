package http

import (
	"net/http"
	"time"

	"github.com/trb1maker/microservices/internal/platform/middleware"
)

const readHeaderTimeout = 5 * time.Second

type HTTPMetrics interface {
	middleware.HTTPInstrumenter
	Handler() http.Handler
}

type ServerConfig struct {
	Addr        string
	ServiceName string
	MetricsPath string
	JWTSecret   string
}

func NewServer(cfg ServerConfig, handler *Handler, httpMetrics HTTPMetrics) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handler.Health)
	mux.HandleFunc("GET /ready", handler.Ready)
	mux.HandleFunc("GET /receipts/{order_id}", handler.GetReceipt)
	mux.HandleFunc("GET /receipts/search", handler.SearchReceipts)

	metricsPath := cfg.MetricsPath
	if metricsPath == "" {
		metricsPath = "/metrics"
	}
	if httpMetrics != nil {
		mux.Handle("GET "+metricsPath, httpMetrics.Handler())
	}

	authSkip := func(r *http.Request) bool {
		switch r.URL.Path {
		case "/health", "/ready", metricsPath:
			return true
		default:
			return false
		}
	}

	httpHandler := middleware.ChainWithAuth(
		mux,
		cfg.ServiceName,
		httpMetrics,
		nil,
		cfg.JWTSecret,
		authSkip,
		nil,
		nil,
		metricsPath,
		nil,
	)

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpHandler,
		ReadHeaderTimeout: readHeaderTimeout,
	}
}
