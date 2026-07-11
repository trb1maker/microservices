package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

const minJWTSecretLength = 32

var (
	ErrJWTSecretRequired   = errors.New("JWT_SECRET is required")
	ErrJWTSecretTooShort   = errors.New("JWT_SECRET must be at least 32 characters")
	ErrTLSCertRequired     = errors.New("TLS_CERT_FILE and TLS_KEY_FILE are required")
	ErrTLSClientCARequired = errors.New("TLS_CLIENT_CA_FILE is required")
	ErrNATSTLSRequired     = errors.New("NATS_TLS_CERT_FILE, NATS_TLS_KEY_FILE and NATS_TLS_CA_FILE are required when USE_MEMORY=false")
)

type Config struct {
	HTTPAddr    string `env:"HTTP_ADDR" envDefault:":8080"`
	DatabaseURL string `env:"DATABASE_URL"`
	RedisAddr   string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	NATSURL     string `env:"NATS_URL" envDefault:"tls://localhost:4222"`
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

	JWTSecret       string        `env:"JWT_SECRET"`
	JWTTTL          time.Duration `env:"JWT_TTL" envDefault:"24h"`
	TLSCertFile     string        `env:"TLS_CERT_FILE"`
	TLSKeyFile      string        `env:"TLS_KEY_FILE"`
	TLSClientCAFile string        `env:"TLS_CLIENT_CA_FILE"`
	NATSTLSCertFile string        `env:"NATS_TLS_CERT_FILE"`
	NATSTLSKeyFile  string        `env:"NATS_TLS_KEY_FILE"`
	NATSTLSCAFile   string        `env:"NATS_TLS_CA_FILE"`
	MTLSServiceCNs  string        `env:"MTLS_SERVICE_CNS" envDefault:"order-service,payment-service"`
}

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
	if c.JWTSecret == "" {
		return ErrJWTSecretRequired
	}

	if len(c.JWTSecret) < minJWTSecretLength {
		return ErrJWTSecretTooShort
	}

	if c.TLSCertFile == "" || c.TLSKeyFile == "" {
		return ErrTLSCertRequired
	}

	if c.TLSClientCAFile == "" {
		return ErrTLSClientCARequired
	}

	if !c.UseMemory && (c.NATSTLSCertFile == "" || c.NATSTLSKeyFile == "" || c.NATSTLSCAFile == "") {
		return ErrNATSTLSRequired
	}

	return nil
}

func (c *Config) ServiceCNs() map[string]struct{} {
	result := make(map[string]struct{})
	for cn := range strings.SplitSeq(c.MTLSServiceCNs, ",") {
		cn = strings.TrimSpace(cn)
		if cn == "" {
			continue
		}

		result[cn] = struct{}{}
	}

	return result
}
