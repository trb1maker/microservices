package metrics

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultMetricsPath   = "/metrics"
	metricsReadTimeout   = 5 * time.Second
	metricsShutdownGrace = 5 * time.Second
)

// Server exposes Prometheus metrics over HTTP.
type Server struct {
	registry    *prometheus.Registry
	metricsPath string
}

// NewServer creates a metrics server with a dedicated registry and Go/process collectors.
func NewServer(namespace, metricsPath string) *Server {
	if metricsPath == "" {
		metricsPath = defaultMetricsPath
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{Namespace: namespace}),
	)

	return &Server{
		registry:    registry,
		metricsPath: metricsPath,
	}
}

// Register adds collectors to the metrics registry.
func (s *Server) Register(collectors ...prometheus.Collector) {
	s.registry.MustRegister(collectors...)
}

// Handler returns the HTTP handler for the metrics endpoint.
func (s *Server) Handler() http.Handler {
	return s.Mux()
}

// Mux returns the underlying HTTP mux so callers can register extra routes.
func (s *Server) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET "+s.metricsPath, promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	return mux
}

// ListenAndServe starts a background HTTP server for metrics scraping.
func (s *Server) ListenAndServe(ctx context.Context, addr string) (*http.Server, error) {
	return s.ListenAndServeWithMux(ctx, addr, s.Mux())
}

// ListenAndServeWithMux starts a background HTTP server using the provided mux.
func (s *Server) ListenAndServeWithMux(ctx context.Context, addr string, mux *http.ServeMux) (*http.Server, error) {
	if mux == nil {
		mux = s.Mux()
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: metricsReadTimeout,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("metrics server failed", slog.Any("error", err))
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), metricsShutdownGrace)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("metrics server shutdown failed", slog.Any("error", err))
		}
	}()

	return server, nil
}

// Path returns the configured metrics path.
func (s *Server) Path() string {
	return s.metricsPath
}
