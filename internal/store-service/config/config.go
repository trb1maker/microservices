package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	NATSURL     string `env:"NATS_URL" envDefault:"tls://localhost:4222"`
	MongoDBURI  string `env:"MONGODB_URI" envDefault:"mongodb://localhost:27017"`
	MongoDBName string `env:"MONGODB_DB_NAME" envDefault:"store"`
	RedisAddr   string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat   string `env:"LOG_FORMAT" envDefault:"json"`

	ServiceName     string `env:"OTEL_SERVICE_NAME" envDefault:"store-service"`
	OTLPEndpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:"http://localhost:4318"`
	OTELSDKDisabled bool   `env:"OTEL_SDK_DISABLED" envDefault:"false"`
	MetricsPath     string `env:"METRICS_PATH" envDefault:"/metrics"`
	MetricsAddr     string `env:"METRICS_ADDR" envDefault:":9092"`

	// NATS subjects (incoming commands)
	ReserveItemsSubject       string `env:"RESERVE_ITEMS_SUBJECT" envDefault:"cart.reserve_items"`
	ConfirmOrderSubject       string `env:"CONFIRM_ORDER_SUBJECT" envDefault:"orders.confirm"`
	ReleaseReservationSubject string `env:"RELEASE_RESERVATION_SUBJECT" envDefault:"cart.release_reservation"`

	// NATS subjects (outgoing events)
	ItemsReservedSubject       string `env:"ITEMS_RESERVED_SUBJECT" envDefault:"store.items_reserved"`
	ReservationFailedSubject   string `env:"RESERVATION_FAILED_SUBJECT" envDefault:"store.reservation_failed"`
	OrderConfirmedSubject      string `env:"ORDER_CONFIRMED_SUBJECT" envDefault:"store.order_confirmed"`
	ReservationReleasedSubject string `env:"RESERVATION_RELEASED_SUBJECT" envDefault:"store.reservation_released"`

	// TLS
	NATSTLSCertFile string `env:"NATS_TLS_CERT_FILE"`
	NATSTLSKeyFile  string `env:"NATS_TLS_KEY_FILE"`
	NATSTLSCAFile   string `env:"NATS_TLS_CA_FILE"`

	StockLockTTL        time.Duration `env:"STOCK_LOCK_TTL" envDefault:"5s"`
	StockLockRetryCount int           `env:"STOCK_LOCK_RETRY_COUNT" envDefault:"20"`
	StockLockRetryDelay time.Duration `env:"STOCK_LOCK_RETRY_DELAY" envDefault:"50ms"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	return &cfg, nil
}
