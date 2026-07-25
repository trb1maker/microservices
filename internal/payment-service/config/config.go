package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	GRPCAddr    string `env:"GRPC_ADDR" envDefault:":50051"`
	DatabaseURL string `env:"DATABASE_URL"`
	NATSURL     string `env:"NATS_URL" envDefault:"tls://localhost:4222"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat   string `env:"LOG_FORMAT" envDefault:"json"`

	ServiceName     string        `env:"OTEL_SERVICE_NAME" envDefault:"payment-service"`
	OTLPEndpoint    string        `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:"http://localhost:4318"`
	OTELSDKDisabled bool          `env:"OTEL_SDK_DISABLED" envDefault:"false"`
	MetricsPath     string        `env:"METRICS_PATH" envDefault:"/metrics"`
	MetricsAddr     string        `env:"METRICS_ADDR" envDefault:":9091"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`

	// NATS subjects
	PaymentSucceededSubject string `env:"PAYMENT_SUCCEEDED_SUBJECT" envDefault:"payment.succeeded"`
	PaymentFailedSubject    string `env:"PAYMENT_FAILED_SUBJECT" envDefault:"payment.failed"`
	RefundSucceededSubject  string `env:"REFUND_SUCCEEDED_SUBJECT" envDefault:"payment.refund_succeeded"`
	RefundFailedSubject     string `env:"REFUND_FAILED_SUBJECT" envDefault:"payment.refund_failed"`

	// TLS
	TLSCertFile     string `env:"TLS_CERT_FILE"`
	TLSKeyFile      string `env:"TLS_KEY_FILE"`
	TLSClientCAFile string `env:"TLS_CLIENT_CA_FILE"`
	NATSTLSCertFile string `env:"NATS_TLS_CERT_FILE"`
	NATSTLSKeyFile  string `env:"NATS_TLS_KEY_FILE"`
	NATSTLSCAFile   string `env:"NATS_TLS_CA_FILE"`
}

var (
	ErrTLSCertRequired     = errors.New("TLS_CERT_FILE and TLS_KEY_FILE are required")
	ErrTLSClientCARequired = errors.New("TLS_CLIENT_CA_FILE is required")
	ErrNATSTLSRequired     = errors.New("NATS_TLS_CERT_FILE, NATS_TLS_KEY_FILE and NATS_TLS_CA_FILE are required")
)

func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.TLSCertFile == "" || c.TLSKeyFile == "" {
		return ErrTLSCertRequired
	}

	if c.TLSClientCAFile == "" {
		return ErrTLSClientCARequired
	}

	if c.NATSTLSCertFile == "" || c.NATSTLSKeyFile == "" || c.NATSTLSCAFile == "" {
		return ErrNATSTLSRequired
	}

	return nil
}
