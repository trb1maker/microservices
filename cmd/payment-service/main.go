package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpchealth "google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/trb1maker/microservices/internal/payment-service/adapters/eventpublisher"
	grpcadapter "github.com/trb1maker/microservices/internal/payment-service/adapters/grpc"
	"github.com/trb1maker/microservices/internal/payment-service/adapters/postgres"
	"github.com/trb1maker/microservices/internal/payment-service/app"
	"github.com/trb1maker/microservices/internal/payment-service/config"
	"github.com/trb1maker/microservices/internal/payment-service/migrations"
	paymentpb "github.com/trb1maker/microservices/internal/platform/proto/payment"

	"github.com/trb1maker/microservices/internal/platform/grpcx"
	"github.com/trb1maker/microservices/internal/platform/health"
	"github.com/trb1maker/microservices/internal/platform/logging"
	"github.com/trb1maker/microservices/internal/platform/metrics"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	pkgotel "github.com/trb1maker/microservices/internal/platform/otel"
	"github.com/trb1maker/microservices/internal/platform/tlsutil"

	"github.com/prometheus/client_golang/prometheus"
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

	pool, err := initPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("init postgres: %w", err)
	}
	defer pool.Close()

	nc, err := initNATS(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init nats: %w", err)
	}
	defer nc.Conn().Close()

	paymentSvc := initPaymentService(pool, nc, cfg)

	metricsServer, grpcRequests, err := initMetrics(ctx, cfg, pool, nc)
	if err != nil {
		return fmt.Errorf("init metrics: %w", err)
	}
	_ = metricsServer

	grpcServer, grpcHealth, err := newGRPCServer(cfg, paymentSvc, grpcRequests)
	if err != nil {
		return fmt.Errorf("create gRPC server: %w", err)
	}
	grpcHealth.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpcHealth.SetServingStatus("payment.PaymentService", grpc_health_v1.HealthCheckResponse_SERVING)

	return serveGRPC(ctx, cfg.GRPCAddr, grpcServer, shutdownTracer, logger)
}

func initPostgres(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(db); err != nil {
		pool.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	if err := db.Close(); err != nil {
		slog.Warn("close migration db", slog.Any("error", err))
	}

	return pool, nil
}

func initNATS(ctx context.Context, cfg *config.Config) (*natsx.Client, error) {
	client, err := natsx.Connect(ctx, natsx.ConnectConfig{
		URL:      cfg.NATSURL,
		Name:     "payment-service",
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

func initPaymentService(pool *pgxpool.Pool, client *natsx.Client, cfg *config.Config) *app.PaymentService {
	accountRepo := postgres.NewAccountRepository(pool)
	transactionRepo := postgres.NewTransactionRepository(pool)

	eventPub := eventpublisher.NewNATSEventPublisher(
		client,
		cfg.PaymentSucceededSubject,
		cfg.PaymentFailedSubject,
		cfg.RefundSucceededSubject,
		cfg.RefundFailedSubject,
	)

	return app.NewPaymentService(accountRepo, transactionRepo, eventPub)
}

func initMetrics(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, client *natsx.Client) (*metrics.Server, *prometheus.CounterVec, error) {
	metricsServer := metrics.NewServer("payment_service", cfg.MetricsPath)
	grpcRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "payment_service",
		Name:      "grpc_requests_total",
		Help:      "Total number of gRPC requests handled by payment-service.",
	}, []string{"method", "status"})
	metricsServer.Register(grpcRequests)

	mux := metricsServer.Mux()
	health.Mount(mux, health.NewChecker(map[string]health.CheckFunc{
		"postgres": func(ctx context.Context) error {
			return pool.Ping(ctx)
		},
		"nats": func(context.Context) error {
			if !client.Conn().IsConnected() {
				return errNATSNotConnected
			}
			return nil
		},
	}))

	if _, err := metricsServer.ListenAndServeWithMux(ctx, cfg.MetricsAddr, mux); err != nil {
		return nil, nil, fmt.Errorf("listen metrics: %w", err)
	}

	slog.Info("metrics server started", slog.String("addr", cfg.MetricsAddr), slog.String("path", cfg.MetricsPath))
	return metricsServer, grpcRequests, nil
}

func serveGRPC(
	ctx context.Context,
	addr string,
	grpcServer *grpc.Server,
	shutdownTracer func(context.Context) error,
	logger *slog.Logger,
) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen gRPC: %w", err)
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("gRPC server started", slog.String("addr", addr))
		if err := grpcServer.Serve(lis); err != nil {
			serverErr <- fmt.Errorf("gRPC serve: %w", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case <-stop:
		slog.Info("shutting down...")
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	grpcServer.GracefulStop()

	if err := shutdownTracer(shutdownCtx); err != nil {
		slog.Error("tracer shutdown error", slog.Any("error", err))
	}

	logger.Info("server stopped")
	return nil
}

func newGRPCServer(cfg *config.Config, paymentSvc *app.PaymentService, grpcRequests *prometheus.CounterVec) (*grpc.Server, *grpchealth.Server, error) {
	tlsConfig, err := tlsutil.LoadMTLSServerTLSConfig(cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSClientCAFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load mtls: %w", err)
	}

	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpcx.ServerStatsHandler(),
		grpc.UnaryInterceptor(grpcx.MetricsUnaryInterceptor(grpcRequests)),
	)

	paymentServer := grpcadapter.NewPaymentServer(paymentSvc)
	paymentpb.RegisterPaymentServiceServer(srv, paymentServer)

	grpcHealth := grpchealth.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, grpcHealth)

	reflection.Register(srv)

	return srv, grpcHealth, nil
}
