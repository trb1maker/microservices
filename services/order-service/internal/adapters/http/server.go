package http

import (
	"crypto/x509"
	"net/http"
	"time"

	"github.com/trb1maker/microservices/pkg/middleware"
)

const readHeaderTimeout = 5 * time.Second

type ServerConfig struct {
	Addr        string
	ServiceName string
	MetricsPath string
	TLSConfig   *TLSConfig
	Auth        *AuthConfig
}

type TLSConfig struct {
	CertFile string
	KeyFile  string
}

type AuthConfig struct {
	JWTSecret  string
	ServiceCAs *x509.CertPool
	ServiceCNs map[string]struct{}
}

func NewServer(
	cfg ServerConfig,
	handler *Handler,
	authHandler AuthService,
	httpMetrics HTTPMetrics,
) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handler.Health)
	mux.HandleFunc("GET /ready", handler.Ready)

	if authHandler != nil {
		login := &loginHandler{auth: authHandler}
		mux.HandleFunc("POST /auth/login", login.Login)
	}

	mux.HandleFunc("POST /cart/items", handler.AddCartItem)
	mux.HandleFunc("GET /cart", handler.GetCart)
	mux.HandleFunc("DELETE /cart/items/{productID}", handler.RemoveCartItem)
	mux.HandleFunc("POST /orders", handler.Checkout)
	mux.HandleFunc("GET /orders/{id}", handler.GetOrder)
	mux.HandleFunc("DELETE /orders/{id}", handler.CancelOrder)
	mux.HandleFunc("GET /debug/error", handler.DebugError)

	metricsPath := cfg.MetricsPath
	if metricsPath == "" {
		metricsPath = "/metrics"
	}

	if httpMetrics != nil {
		mux.Handle("GET "+metricsPath, httpMetrics.Handler())
	}

	var secret string
	var serviceCAs *x509.CertPool
	var serviceCNs map[string]struct{}
	if cfg.Auth != nil {
		secret = cfg.Auth.JWTSecret
		serviceCAs = cfg.Auth.ServiceCAs
		serviceCNs = cfg.Auth.ServiceCNs
	}

	httpHandler := middleware.ChainWithAuth(
		mux, cfg.ServiceName, httpMetrics, nil,
		secret, makePublicPathSkipper(metricsPath), serviceCAs, serviceCNs, metricsPath,
	)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpHandler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return server
}

func makePublicPathSkipper(metricsPath string) middleware.AuthSkip {
	return func(r *http.Request) bool {
		switch r.URL.Path {
		case "/health", "/ready", metricsPath, "/auth/login":
			return true
		default:
			return false
		}
	}
}
