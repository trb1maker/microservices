package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/trb1maker/microservices/pkg/httpx"
	"github.com/trb1maker/microservices/pkg/logging"
)

// AccessLogSkip определяет, нужно ли пропустить access log для запроса.
type AccessLogSkip func(*http.Request) bool

// Middleware оборачивает http.Handler.
type Middleware func(http.Handler) http.Handler

func AccessLog(skip AccessLogSkip) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip != nil && skip(r) {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			recorder := &httpx.StatusRecorder{ResponseWriter: w, Status: http.StatusOK}
			next.ServeHTTP(recorder, r)

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", recorder.Status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			}

			if traceID := logging.TraceIDFromContext(r.Context()); traceID != "" {
				attrs = append(attrs, slog.String("trace_id", traceID))
			}

			if route := r.Pattern; route != "" {
				attrs = append(attrs, slog.String("route", route))
			}

			slog.LogAttrs(r.Context(), slog.LevelInfo, "http_request", attrs...)
		})
	}
}
