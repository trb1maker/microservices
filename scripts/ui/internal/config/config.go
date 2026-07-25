package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTPAddr         string
	OrderHTTPBaseURL string
	OrderGRPCAddr    string
	TLSCAFile        string
	TLSSkipVerify    bool
	SessionSecret    string
	SessionMaxAge    time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:         envOr("UI_HTTP_ADDR", ":8081"),
		OrderHTTPBaseURL: envOr("ORDER_HTTP_URL", "https://localhost:8080"),
		OrderGRPCAddr:    envOr("ORDER_GRPC_ADDR", "localhost:50052"),
		TLSCAFile:        os.Getenv("TLS_CA_FILE"),
		TLSSkipVerify:    os.Getenv("TLS_SKIP_VERIFY") == "true",
		SessionSecret:    envOr("UI_SESSION_SECRET", "dev-ui-session-secret-minimum-32-chars"),
		SessionMaxAge:    24 * time.Hour,
	}
	if len(cfg.SessionSecret) < 32 {
		return nil, fmt.Errorf("UI_SESSION_SECRET must be at least 32 characters")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
