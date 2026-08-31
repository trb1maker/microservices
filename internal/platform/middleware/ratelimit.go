package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const rateLimitKeyPrefix = "rl:"

const rateLimitScript = `
local count = redis.call("INCR", KEYS[1])
if count == 1 then
	redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return count
`

// RateLimitConfig configures Redis-backed request rate limiting.
type RateLimitConfig struct {
	Client  *goredis.Client
	Limit   int
	Window  time.Duration
	Skip    func(*http.Request) bool
	OnLimit func()
}

// RateLimit returns middleware that limits requests per identifier.
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	if cfg.Limit <= 0 || cfg.Client == nil || cfg.Window <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	limiter := cfg

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter.Skip != nil && limiter.Skip(r) {
				next.ServeHTTP(w, r)
				return
			}

			if !rateLimitRequest(w, r, limiter, next) {
				return
			}
		})
	}
}

func rateLimitRequest(w http.ResponseWriter, r *http.Request, cfg RateLimitConfig, next http.Handler) bool {
	identifier, ok := rateLimitIdentifier(r)
	if !ok {
		next.ServeHTTP(w, r)
		return true
	}

	allowed, retryAfter, err := allowRequest(r.Context(), cfg, identifier)
	if err != nil {
		http.Error(w, "rate limiter unavailable", http.StatusServiceUnavailable)
		return false
	}

	if !allowed {
		if cfg.OnLimit != nil {
			cfg.OnLimit()
		}
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		}
		writeRateLimited(w)
		return false
	}

	next.ServeHTTP(w, r)
	return true
}

func allowRequest(ctx context.Context, cfg RateLimitConfig, identifier string) (bool, int, error) {
	windowStart := time.Now().UTC().Truncate(cfg.Window).Unix()
	key := fmt.Sprintf("%s%s:%d", rateLimitKeyPrefix, identifier, windowStart)
	windowSec := int64(cfg.Window.Seconds())
	if windowSec <= 0 {
		windowSec = 1
	}

	count, err := cfg.Client.Eval(ctx, rateLimitScript, []string{key}, windowSec).Int64()
	if err != nil {
		return false, 0, fmt.Errorf("increment rate limit counter: %w", err)
	}

	if count > int64(cfg.Limit) {
		ttl, err := cfg.Client.TTL(ctx, key).Result()
		if err != nil {
			return false, int(cfg.Window.Seconds()), nil
		}
		retryAfter := max(int(ttl.Seconds()), 1)
		return false, retryAfter, nil
	}

	return true, 0, nil
}

func rateLimitIdentifier(r *http.Request) (string, bool) {
	if userID, ok := UserIDFromContext(r.Context()); ok {
		return "user:" + userID.String(), true
	}

	if ip := clientIP(r); ip != "" {
		return "ip:" + ip, true
	}

	return "", false
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

func writeRateLimited(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
}

// SkipRateLimitPaths returns a Skip func that bypasses rate limiting for health and metrics endpoints.
func SkipRateLimitPaths(metricsPath string) func(*http.Request) bool {
	if metricsPath == "" {
		metricsPath = "/metrics"
	}

	return func(r *http.Request) bool {
		switch r.URL.Path {
		case "/health", "/ready", metricsPath:
			return true
		default:
			return false
		}
	}
}
