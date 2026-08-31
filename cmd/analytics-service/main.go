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

	docpostgres "github.com/trb1maker/microservices/internal/analytics-service/adapters/document_repository/postgres"
	natsconsumer "github.com/trb1maker/microservices/internal/analytics-service/adapters/event_consumer/nats"
	analyticshttp "github.com/trb1maker/microservices/internal/analytics-service/adapters/http"
	minioadapter "github.com/trb1maker/microservices/internal/analytics-service/adapters/receipt_storage/minio"
	summarypostgres "github.com/trb1maker/microservices/internal/analytics-service/adapters/summary_repository/postgres"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
	"github.com/trb1maker/microservices/internal/analytics-service/config"
	"github.com/trb1maker/microservices/internal/analytics-service/migrations"

	"github.com/trb1maker/microservices/internal/platform/health"
	"github.com/trb1maker/microservices/internal/platform/logging"
	"github.com/trb1maker/microservices/internal/platform/metrics"
	"github.com/trb1maker/microservices/internal/platform/natsx"
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

	runtime, err := bootstrap(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer runtime.pool.Close()
	defer runtime.natsClient.Conn().Close()
	defer runtime.consumer.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-runtime.apiErr:
		return fmt.Errorf("http server: %w", err)
	case <-stop:
	}

	return shutdownAll(ctx, runtime, shutdownTracer, logger)
}

type appRuntime struct {
	pool       *pgxpool.Pool
	natsClient *natsx.Client
	consumer   *natsconsumer.Consumer
	apiServer  *http.Server
	apiErr     chan error
}

func bootstrap(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*appRuntime, error) {
	pool, summaryRepo, documentRepo, err := initPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("init postgres: %w", err)
	}

	receiptStorage, err := minioadapter.NewStorage(minioadapter.Config{
		Endpoint:       cfg.MinIOEndpoint,
		PublicEndpoint: cfg.MinIOPublicEndpoint,
		AccessKey:      cfg.MinIOAccessKey,
		SecretKey:      cfg.MinIOSecretKey,
		Bucket:         cfg.MinIOBucket,
		UseSSL:         cfg.MinIOUseSSL,
		PublicUseSSL:   cfg.MinIOPublicUseSSL,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("init minio: %w", err)
	}

	natsClient, err := initNATS(ctx, cfg)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("init nats: %w", err)
	}

	readiness := health.NewChecker(map[string]health.CheckFunc{
		"postgres": summaryRepo.Ping,
		"minio":    receiptStorage.Ping,
		"nats": func(context.Context) error {
			if !natsClient.Conn().IsConnected() {
				return errNATSNotConnected
			}
			return nil
		},
	})

	metricsServer := metrics.NewServer("analytics_service", cfg.MetricsPath)
	if _, err := startHealthServer(ctx, metricsServer, readiness, cfg.MetricsAddr); err != nil {
		pool.Close()
		natsClient.Conn().Close()
		return nil, fmt.Errorf("start metrics server: %w", err)
	}

	svc := app.NewAnalyticsService(receiptStorage, summaryRepo, documentRepo, cfg.ReceiptURLTTL)
	httpHandler := analyticshttp.NewHandler(svc, readinessAdapter{checker: readiness})
	apiServer := analyticshttp.NewServer(analyticshttp.ServerConfig{
		Addr:        cfg.HTTPAddr,
		ServiceName: cfg.ServiceName,
		MetricsPath: cfg.MetricsPath,
		JWTSecret:   cfg.JWTSecret,
	}, httpHandler, metrics.New(cfg.MetricsPath))

	apiErr := make(chan error, 1)
	go func() {
		logger.Info("http server started", slog.String("addr", cfg.HTTPAddr))
		if err := apiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			apiErr <- err
		}
	}()

	consumer := natsconsumer.NewConsumer(natsClient, cfg.OrderFinalizedSubject, svc)
	if err := consumer.Start(ctx); err != nil {
		pool.Close()
		natsClient.Conn().Close()
		return nil, fmt.Errorf("start consumer: %w", err)
	}

	slog.Info("analytics-service started")
	return &appRuntime{
		pool:       pool,
		natsClient: natsClient,
		consumer:   consumer,
		apiServer:  apiServer,
		apiErr:     apiErr,
	}, nil
}

func shutdownAll(
	ctx context.Context,
	runtime *appRuntime,
	shutdownTracer func(context.Context) error,
	logger *slog.Logger,
) error {
	slog.Info("shutting down...")
	shutdownCtx, cancelShutdown := context.WithTimeout(ctx, shutdownTimeout)
	defer cancelShutdown()
	if err := runtime.apiServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown error", slog.Any("error", err))
	}
	if err := shutdownTracer(shutdownCtx); err != nil {
		slog.Error("tracer shutdown error", slog.Any("error", err))
	}
	logger.Info("server stopped")
	return nil
}

type readinessAdapter struct {
	checker *health.Checker
}

func (r readinessAdapter) Check(ctx context.Context) (bool, map[string]string) {
	return r.checker.Check(ctx)
}

func startHealthServer(
	ctx context.Context,
	metricsServer *metrics.Server,
	readiness *health.Checker,
	addr string,
) (*http.Server, error) {
	mux := metricsServer.Mux()
	mux.HandleFunc("GET /health", health.LivenessHandler())
	mux.HandleFunc("GET /ready", health.ReadinessHandler(readiness))
	server, err := metricsServer.ListenAndServeWithMux(ctx, addr, mux)
	if err != nil {
		return nil, fmt.Errorf("listen metrics: %w", err)
	}
	return server, nil
}

func initPostgres(
	ctx context.Context,
	databaseURL string,
) (*pgxpool.Pool, *summarypostgres.SummaryRepository, *docpostgres.DocumentRepository, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect postgres: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(db); err != nil {
		pool.Close()
		return nil, nil, nil, fmt.Errorf("migrate postgres: %w", err)
	}
	if err := db.Close(); err != nil {
		slog.Warn("close migration db", slog.Any("error", err))
	}
	return pool, summarypostgres.NewSummaryRepository(pool), docpostgres.NewDocumentRepository(pool), nil
}

func initNATS(ctx context.Context, cfg *config.Config) (*natsx.Client, error) {
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
	client, err := natsx.New(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("init jetstream: %w", err)
	}
	slog.Info("connected to NATS", slog.String("url", cfg.NATSURL))
	return client, nil
}
