package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	natsconsumer "github.com/trb1maker/microservices/internal/analytics-service/adapters/event_consumer/nats"
	minioadapter "github.com/trb1maker/microservices/internal/analytics-service/adapters/receipt_storage/minio"
	summarypostgres "github.com/trb1maker/microservices/internal/analytics-service/adapters/summary_repository/postgres"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
	"github.com/trb1maker/microservices/internal/analytics-service/config"
	"github.com/trb1maker/microservices/internal/analytics-service/migrations"

	"github.com/trb1maker/microservices/internal/platform/health"
	"github.com/trb1maker/microservices/internal/platform/logging"
	"github.com/trb1maker/microservices/internal/platform/metrics"
	pkgotel "github.com/trb1maker/microservices/internal/platform/otel"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
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

	pool, summaryRepo, err := initPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("init postgres: %w", err)
	}
	defer pool.Close()

	receiptStorage, err := minioadapter.NewStorage(minioadapter.Config{
		Endpoint:  cfg.MinIOEndpoint,
		AccessKey: cfg.MinIOAccessKey,
		SecretKey: cfg.MinIOSecretKey,
		Bucket:    cfg.MinIOBucket,
		UseSSL:    cfg.MinIOUseSSL,
	})
	if err != nil {
		return fmt.Errorf("init minio: %w", err)
	}

	nc, err := initNATS(cfg)
	if err != nil {
		return fmt.Errorf("init nats: %w", err)
	}
	defer nc.Close()

	metricsServer := metrics.NewServer("analytics_service", cfg.MetricsPath)
	if _, err := startHealthServer(ctx, metricsServer, summaryRepo, nc, cfg.MetricsAddr); err != nil {
		return fmt.Errorf("start metrics server: %w", err)
	}

	svc := app.NewAnalyticsService(receiptStorage, summaryRepo)
	consumer := natsconsumer.NewConsumer(nc, cfg.OrderFinalizedSubject, svc)
	if err := consumer.Start(); err != nil {
		return fmt.Errorf("start consumer: %w", err)
	}
	defer consumer.Close()

	slog.Info("analytics-service started, waiting for NATS messages")

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

func startHealthServer(
	ctx context.Context,
	metricsServer *metrics.Server,
	summaryRepo *summarypostgres.SummaryRepository,
	nc *nats.Conn,
	addr string,
) (*http.Server, error) {
	mux := metricsServer.Mux()
	mux.HandleFunc("GET /health", health.LivenessHandler())
	mux.HandleFunc("GET /ready", health.ReadinessHandler(health.NewChecker(map[string]health.CheckFunc{
		"postgres": summaryRepo.Ping,
		"nats": func(context.Context) error {
			if !nc.IsConnected() {
				return errNATSNotConnected
			}
			return nil
		},
	})))
	server, err := metricsServer.ListenAndServeWithMux(ctx, addr, mux)
	if err != nil {
		return nil, fmt.Errorf("listen metrics: %w", err)
	}
	return server, nil
}

func initPostgres(ctx context.Context, databaseURL string) (*pgxpool.Pool, *summarypostgres.SummaryRepository, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect postgres: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(db); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("migrate postgres: %w", err)
	}
	if err := db.Close(); err != nil {
		slog.Warn("close migration db", slog.Any("error", err))
	}
	return pool, summarypostgres.NewSummaryRepository(pool), nil
}

func initNATS(cfg *config.Config) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name("analytics-service"),
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
