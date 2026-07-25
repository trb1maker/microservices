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

	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/trb1maker/microservices/internal/store-service/adapters/mongodb"
	natsadapter "github.com/trb1maker/microservices/internal/store-service/adapters/nats"
	"github.com/trb1maker/microservices/internal/store-service/app"
	"github.com/trb1maker/microservices/internal/store-service/config"

	"github.com/trb1maker/microservices/internal/platform/health"
	"github.com/trb1maker/microservices/internal/platform/logging"
	"github.com/trb1maker/microservices/internal/platform/metrics"
	pkgotel "github.com/trb1maker/microservices/internal/platform/otel"
)

const (
	shutdownTimeout           = 10 * time.Second
	expectedNATSSubscriptions = 3
)

var (
	errNATSNotConnected          = errors.New("nats is not connected")
	errInsufficientSubscriptions = errors.New("insufficient nats subscriptions")
)

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

	client, db, err := initMongo(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init mongodb: %w", err)
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			slog.Error("mongodb disconnect error", slog.Any("error", err))
		}
	}()

	nc, err := initNATS(cfg)
	if err != nil {
		return fmt.Errorf("init nats: %w", err)
	}
	defer nc.Close()

	storeSvc, worker, err := initStoreService(db, nc, cfg)
	if err != nil {
		return fmt.Errorf("init store service: %w", err)
	}

	metricsServer := metrics.NewServer("store_service", cfg.MetricsPath)
	mux := metricsServer.Mux()
	mux.HandleFunc("GET /health", health.LivenessHandler())
	mux.HandleFunc("GET /ready", health.ReadinessHandler(health.NewChecker(map[string]health.CheckFunc{
		"mongodb": func(ctx context.Context) error {
			return client.Ping(ctx, nil)
		},
		"nats": func(context.Context) error {
			if !nc.IsConnected() {
				return errNATSNotConnected
			}
			return nil
		},
		"nats-subscriptions": func(context.Context) error {
			if worker.ActiveSubscriptions() < expectedNATSSubscriptions {
				return errInsufficientSubscriptions
			}
			return nil
		},
	})))
	if _, err := metricsServer.ListenAndServeWithMux(ctx, cfg.MetricsAddr, mux); err != nil {
		return fmt.Errorf("start metrics server: %w", err)
	}
	slog.Info("metrics server started", slog.String("addr", cfg.MetricsAddr), slog.String("path", cfg.MetricsPath))

	return serveWorker(ctx, nc, worker, storeSvc, shutdownTracer, logger)
}

func initMongo(ctx context.Context, cfg *config.Config) (*mongo.Client, *mongo.Database, error) {
	clientOpts := options.Client().ApplyURI(cfg.MongoDBURI)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to mongodb: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, nil, fmt.Errorf("ping mongodb: %w", err)
	}

	slog.Info("connected to MongoDB", slog.String("uri", cfg.MongoDBURI))

	db := client.Database(cfg.MongoDBName)
	if err := mongodb.SeedProducts(ctx, db); err != nil {
		_ = client.Disconnect(ctx)
		return nil, nil, fmt.Errorf("seed products: %w", err)
	}

	slog.Info("products and stock seeded")
	return client, db, nil
}

func initNATS(cfg *config.Config) (*nats.Conn, error) {
	natsOpts := []nats.Option{
		nats.Name("store-service"),
		nats.Secure(&tls.Config{
			MinVersion: tls.VersionTLS12,
		}),
	}

	if cfg.NATSTLSCertFile != "" && cfg.NATSTLSKeyFile != "" {
		natsOpts = append(natsOpts,
			nats.ClientCert(cfg.NATSTLSCertFile, cfg.NATSTLSKeyFile),
			nats.RootCAs(cfg.NATSTLSCAFile),
		)
	}

	nc, err := nats.Connect(cfg.NATSURL, natsOpts...)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	slog.Info("connected to NATS", slog.String("url", cfg.NATSURL))
	return nc, nil
}

func initStoreService(db *mongo.Database, nc *nats.Conn, cfg *config.Config) (*app.StoreService, *natsadapter.Worker, error) {
	productRepo := mongodb.NewProductRepository(db)
	stockRepo := mongodb.NewStockRepository(db)

	eventPub := natsadapter.NewEventPublisher(
		nc,
		cfg.ItemsReservedSubject,
		cfg.ReservationFailedSubject,
		cfg.OrderConfirmedSubject,
		cfg.ReservationReleasedSubject,
	)

	storeSvc := app.NewStoreService(productRepo, stockRepo, eventPub)
	worker := natsadapter.NewWorker(storeSvc)
	if err := worker.SubscribeAll(nc, cfg.ReserveItemsSubject, cfg.ConfirmOrderSubject, cfg.ReleaseReservationSubject); err != nil {
		return nil, nil, fmt.Errorf("subscribe to NATS: %w", err)
	}

	return storeSvc, worker, nil
}

func serveWorker(
	ctx context.Context,
	nc *nats.Conn,
	_ *natsadapter.Worker,
	_ *app.StoreService,
	shutdownTracer func(context.Context) error,
	logger *slog.Logger,
) error {
	slog.Info("store-service started, waiting for NATS messages")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop
	slog.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	nc.Close()

	if err := shutdownTracer(shutdownCtx); err != nil {
		slog.Error("tracer shutdown error", slog.Any("error", err))
	}

	logger.Info("server stopped")
	return nil
}
