package middleware

import (
	"crypto/x509"
	"net/http"

	"github.com/trb1maker/microservices/pkg/httpx"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPInstrumenter wraps handlers to record HTTP metrics.
type HTTPInstrumenter interface {
	Instrument(next http.Handler) http.Handler
}

// ChainWithAuth собирает middleware-цепочку с JWT/mTLS auth.
func ChainWithAuth(
	handler http.Handler,
	serviceName string,
	instrumenter HTTPInstrumenter,
	skip AccessLogSkip,
	secret string,
	authSkip AuthSkip,
	serviceCAs *x509.CertPool,
	serviceCNs map[string]struct{},
	metricsPath string,
) http.Handler {
	h := handler

	if secret != "" || serviceCAs != nil {
		h = JWTAuth(secret, authSkip, serviceCAs, serviceCNs)(h)
	}

	return Chain(h, serviceName, instrumenter, skip, metricsPath)
}

// Chain собирает middleware-цепочку: otel (внешний) → access log → metrics → handler.
// Порядок важен: otel должен обернуть access log, чтобы trace_id был в контексте до записи лога.
func Chain(
	handler http.Handler,
	serviceName string,
	instrumenter HTTPInstrumenter,
	skip AccessLogSkip,
	metricsPath string,
) http.Handler {
	if metricsPath == "" {
		metricsPath = "/metrics"
	}
	if skip == nil {
		skip = defaultSkipAccessLog
	}

	h := handler

	if instrumenter != nil {
		h = instrumenter.Instrument(h)
	}

	h = AccessLog(skip)(h)

	h = otelhttp.NewHandler(h, serviceName,
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != metricsPath
		}),
	)

	return h
}

func defaultSkipAccessLog(r *http.Request) bool {
	return httpx.ShouldSkipObservability(r.URL.Path) || r.URL.Path == "/health"
}
