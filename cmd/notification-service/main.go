package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	natsadapter "github.com/trb1maker/microservices/internal/notification-service/adapters/nats"
	"github.com/trb1maker/microservices/internal/notification-service/adapters/notifier"
	"github.com/trb1maker/microservices/internal/notification-service/app"
	"github.com/trb1maker/microservices/internal/notification-service/config"

	"github.com/trb1maker/microservices/internal/platform/health"
	"github.com/trb1maker/microservices/internal/platform/logging"
	"github.com/trb1maker/microservices/internal/platform/metrics"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	pkgotel "github.com/trb1maker/microservices/internal/platform/otel"
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

	nc, err := initNATS(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init nats: %w", err)
	}
	defer nc.Conn().Close()

	metricsServer := metrics.NewServer("notification_service", cfg.MetricsPath)
	mux := metricsServer.Mux()
	health.Mount(mux, health.NewChecker(map[string]health.CheckFunc{
		"nats": func(context.Context) error {
			if !nc.Conn().IsConnected() {
				return errNATSNotConnected
			}
			return nil
		},
	}))
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
	if err := consumer.Start(ctx); err != nil {
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

func initNATS(ctx context.Context, cfg *config.Config) (*natsx.Client, error) {
	client, err := natsx.Connect(ctx, natsx.ConnectConfig{
		URL:      cfg.NATSURL,
		Name:     "notification-service",
		CertFile: cfg.NATSTLSCertFile,
		KeyFile:  cfg.NATSTLSKeyFile,
		CAFile:   cfg.NATSTLSCAFile,
	})
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	slog.Info("connected to NATS", slog.String("url", cfg.NATSURL))
	return client, nil
}
