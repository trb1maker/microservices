package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HTTPAddr    string `env:"HTTP_ADDR" envDefault:":8080"`
	DatabaseURL string `env:"DATABASE_URL"`
	RedisAddr   string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	NATSURL     string `env:"NATS_URL" envDefault:"nats://localhost:4222"`
	UseMemory   bool   `env:"USE_MEMORY" envDefault:"false"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat   string `env:"LOG_FORMAT" envDefault:"json"`

	ServiceName            string `env:"OTEL_SERVICE_NAME" envDefault:"order-service"`
	OTLPEndpoint           string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:"http://localhost:4318"`
	OTELSDKDisabled        bool   `env:"OTEL_SDK_DISABLED" envDefault:"false"`
	MetricsPath            string `env:"METRICS_PATH" envDefault:"/metrics"`
	ActiveOrdersRefreshSec int    `env:"ACTIVE_ORDERS_REFRESH_SEC" envDefault:"30"`

	OrderCreatedSubject       string `env:"ORDER_CREATED_SUBJECT" envDefault:"orders.created"`
	ReserveItemsSubject       string `env:"RESERVE_ITEMS_SUBJECT" envDefault:"cart.reserve_items"`
	ConfirmOrderSubject       string `env:"CONFIRM_ORDER_SUBJECT" envDefault:"orders.confirm"`
	ReleaseReservationSubject string `env:"RELEASE_RESERVATION_SUBJECT" envDefault:"cart.release_reservation"`
	OrderFinalizedSubject     string `env:"ORDER_FINALIZED_SUBJECT" envDefault:"orders.finalized"`
	OrderCancelledSubject     string `env:"ORDER_CANCELLED_SUBJECT" envDefault:"orders.cancelled"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	return &cfg, nil
}
