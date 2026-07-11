package middleware

import (
	"net/http"

	"github.com/trb1maker/microservices/pkg/httpx"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPInstrumenter wraps handlers to record HTTP metrics.
type HTTPInstrumenter interface {
	Instrument(next http.Handler) http.Handler
}

// Chain собирает middleware-цепочку: otel (внешний) → access log → metrics → handler.
// Порядок важен: otel должен обернуть access log, чтобы trace_id был в контексте до записи лога.
func Chain(
	handler http.Handler,
	serviceName string,
	instrumenter HTTPInstrumenter,
	skip AccessLogSkip,
) http.Handler {
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
			return !httpx.ShouldSkipObservability(r.URL.Path)
		}),
	)

	return h
}

func defaultSkipAccessLog(r *http.Request) bool {
	return httpx.ShouldSkipObservability(r.URL.Path) || r.URL.Path == "/health"
}
