package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/trb1maker/microservices/pkg/httpx"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "order_service"

type Metrics struct {
	registry *prometheus.Registry

	metricsPath string

	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	HTTPRequestsInFlight prometheus.Gauge
	OrdersCreatedTotal   prometheus.Counter
	OrdersActive         prometheus.Gauge
}

func New(metricsPath string) *Metrics {
	if metricsPath == "" {
		metricsPath = "/metrics"
	}

	registry := prometheus.NewRegistry()

	m := &Metrics{
		registry:    registry,
		metricsPath: metricsPath,
		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests.",
		}, []string{"method", "route", "status"}),
		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route"}),
		HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "http_requests_in_flight",
			Help:      "Number of HTTP requests currently being served.",
		}),
		OrdersCreatedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "orders_created_total",
			Help:      "Total number of successfully created orders.",
		}),
		OrdersActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "orders_active",
			Help:      "Number of active orders (not confirmed or cancelled).",
		}),
	}

	registry.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.HTTPRequestsInFlight,
		m.OrdersCreatedTotal,
		m.OrdersActive,
	)

	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == m.metricsPath {
			next.ServeHTTP(w, r)
			return
		}

		route := r.Pattern
		if route == "" {
			route = "unknown"
		}

		m.HTTPRequestsInFlight.Inc()
		defer m.HTTPRequestsInFlight.Dec()

		start := time.Now()
		recorder := &httpx.StatusRecorder{ResponseWriter: w, Status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		elapsed := time.Since(start).Seconds()
		status := strconv.Itoa(recorder.Status)

		m.HTTPRequestsTotal.WithLabelValues(r.Method, route, status).Inc()
		m.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(elapsed)
	})
}

func (m *Metrics) RecordOrderCreated() {
	m.OrdersCreatedTotal.Inc()
}

func (m *Metrics) SetActiveOrders(count int) {
	m.OrdersActive.Set(float64(count))
}
