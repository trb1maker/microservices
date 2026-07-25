package config

import (
	"errors"
	"fmt"

	"github.com/caarlos0/env/v11"
)

var ErrDatabaseURLRequired = errors.New("ANALYTICS_DATABASE_URL is required")

type Config struct {
	NATSURL         string `env:"NATS_URL" envDefault:"tls://localhost:4222"`
	DatabaseURL     string `env:"ANALYTICS_DATABASE_URL"`
	LogLevel        string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat       string `env:"LOG_FORMAT" envDefault:"json"`
	ServiceName     string `env:"OTEL_SERVICE_NAME" envDefault:"analytics-service"`
	OTLPEndpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:"http://localhost:4318"`
	OTELSDKDisabled bool   `env:"OTEL_SDK_DISABLED" envDefault:"false"`
	MetricsPath     string `env:"METRICS_PATH" envDefault:"/metrics"`
	MetricsAddr     string `env:"METRICS_ADDR" envDefault:":9094"`

	OrderFinalizedSubject string `env:"ORDER_FINALIZED_SUBJECT" envDefault:"orders.finalized"`

	MinIOEndpoint  string `env:"MINIO_ENDPOINT" envDefault:"localhost:9000"`
	MinIOAccessKey string `env:"MINIO_ACCESS_KEY" envDefault:"minioadmin"`
	MinIOSecretKey string `env:"MINIO_SECRET_KEY" envDefault:"minioadmin"`
	MinIOBucket    string `env:"MINIO_BUCKET" envDefault:"receipts"`
	MinIOUseSSL    bool   `env:"MINIO_USE_SSL" envDefault:"false"`

	NATSTLSCertFile string `env:"NATS_TLS_CERT_FILE"`
	NATSTLSKeyFile  string `env:"NATS_TLS_KEY_FILE"`
	NATSTLSCAFile   string `env:"NATS_TLS_CA_FILE"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return nil, ErrDatabaseURLRequired
	}
	return &cfg, nil
}
