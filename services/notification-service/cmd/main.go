package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	natsadapter "github.com/trb1maker/microservices/services/notification-service/internal/adapters/nats"
	"github.com/trb1maker/microservices/services/notification-service/internal/adapters/notifier"
	"github.com/trb1maker/microservices/services/notification-service/internal/app"
	"github.com/trb1maker/microservices/services/notification-service/internal/config"

	"github.com/trb1maker/microservices/pkg/health"
	"github.com/trb1maker/microservices/pkg/logging"
	"github.com/trb1maker/microservices/pkg/metrics"
	pkgotel "github.com/trb1maker/microservices/pkg/otel"

	"github.com/nats-io/nats.go"
)

const shutdownTimeout = 10 * time.Second

var errNATSNotConnected = errors.New("nats is not connected")

func main() {
	if err := run(); err != nil {
		slog.Error("application failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := logging.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := pkgotel.Init(ctx, cfg.ServiceName, cfg.OTLPEndpoint, cfg.OTELSDKDisabled)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		if err := shutdownTracer(ctx); err != nil {
			slog.Error("tracer shutdown error", slog.Any("error", err))
		}
	}()

	nc, err := initNATS(cfg)
	if err != nil {
		return fmt.Errorf("init nats: %w", err)
	}
	defer nc.Close()

	metricsServer := metrics.NewServer("notification_service", cfg.MetricsPath)
	mux := metricsServer.Mux()
	mux.HandleFunc("GET /health", health.LivenessHandler())
	mux.HandleFunc("GET /ready", health.ReadinessHandler(health.NewChecker(map[string]health.CheckFunc{
		"nats": func(context.Context) error {
			if !nc.IsConnected() {
				return errNATSNotConnected
			}
			return nil
		},
	})))
	if _, err := metricsServer.ListenAndServeWithMux(ctx, cfg.MetricsAddr, mux); err != nil {
		return fmt.Errorf("start metrics server: %w", err)
	}

	svc := app.NewNotificationService(notifier.NewSlogNotifier(cfg.ServiceName))
	consumer := natsadapter.NewConsumer(nc, natsadapter.Subjects{
		OrderFinalized:   cfg.OrderFinalizedSubject,
		OrderCancelled:   cfg.OrderCancelledSubject,
		PaymentSucceeded: cfg.PaymentSucceededSubject,
		RefundSucceeded:  cfg.RefundSucceededSubject,
	}, svc)
	if err := consumer.Start(); err != nil {
		return fmt.Errorf("start consumer: %w", err)
	}
	defer consumer.Close()

	slog.Info("notification-service started, waiting for NATS messages")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down...")

	shutdownCtx, cancelShutdown := context.WithTimeout(ctx, shutdownTimeout)
	defer cancelShutdown()
	if err := shutdownTracer(shutdownCtx); err != nil {
		slog.Error("tracer shutdown error", slog.Any("error", err))
	}
	logger.Info("server stopped")
	return nil
}

func initNATS(cfg *config.Config) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name("notification-service"),
		nats.Secure(&tls.Config{MinVersion: tls.VersionTLS12}),
	}
	if cfg.NATSTLSCertFile != "" && cfg.NATSTLSKeyFile != "" {
		opts = append(opts,
			nats.ClientCert(cfg.NATSTLSCertFile, cfg.NATSTLSKeyFile),
			nats.RootCAs(cfg.NATSTLSCAFile),
		)
	}

	conn, err := nats.Connect(cfg.NATSURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	slog.Info("connected to NATS", slog.String("url", cfg.NATSURL))
	return conn, nil
}
