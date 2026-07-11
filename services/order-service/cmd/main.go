package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	cartmemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/cart_repository/memory"
	cartredis "github.com/trb1maker/microservices/services/order-service/internal/adapters/cart_repository/redis"
	natsadapter "github.com/trb1maker/microservices/services/order-service/internal/adapters/event_publisher/nats"
	httpadapter "github.com/trb1maker/microservices/services/order-service/internal/adapters/http"
	ordermemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/order_repository/memory"
	orderpostgres "github.com/trb1maker/microservices/services/order-service/internal/adapters/order_repository/postgres"
	userpostgres "github.com/trb1maker/microservices/services/order-service/internal/adapters/user_repository/postgres"
	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/internal/config"
	"github.com/trb1maker/microservices/services/order-service/migrations"

	"github.com/trb1maker/microservices/pkg/health"
	"github.com/trb1maker/microservices/pkg/logging"
	"github.com/trb1maker/microservices/pkg/metrics"
	pkgotel "github.com/trb1maker/microservices/pkg/otel"
	"github.com/trb1maker/microservices/pkg/tlsutil"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"
)

const shutdownTimeout = 10 * time.Second

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

	ctx := context.Background()

	shutdownTracer, err := pkgotel.Init(ctx, cfg.ServiceName, cfg.OTLPEndpoint, cfg.OTELSDKDisabled)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}

	appMetrics := metrics.New(cfg.MetricsPath)

	cartRepo, orderRepo, events, readiness, authService, cleanup, err := buildDependencies(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("build dependencies: %w", err)
	}
	defer cleanup()

	startActiveOrdersRefresh(ctx, cfg, appMetrics, orderRepo, logger)

	server, err := newHTTPServer(cfg, cartRepo, orderRepo, events, readiness, authService, appMetrics)
	if err != nil {
		return err
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server started", slog.String("addr", cfg.HTTPAddr), slog.String("scheme", "https"))
		if err := server.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case <-stop:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	if err := shutdownTracer(shutdownCtx); err != nil {
		return fmt.Errorf("tracer shutdown: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

func newHTTPServer(
	cfg *config.Config,
	cartRepo app.CartRepository,
	orderRepo app.OrderRepository,
	events app.EventPublisher,
	readiness *health.Checker,
	authService *app.AuthService,
	appMetrics *metrics.Metrics,
) (*http.Server, error) {
	cartService := app.NewCartService(cartRepo)
	orderService := app.NewOrderService(cartRepo, orderRepo, events, appMetrics)
	handler := httpadapter.NewHandler(cartService, orderService, readiness)

	serviceCAs, err := tlsutil.LoadClientCAPool(cfg.TLSClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("load client ca: %w", err)
	}

	var authHandler httpadapter.AuthService
	if authService != nil {
		authHandler = httpadapter.NewAppAuthAdapter(authService)
	}

	server := httpadapter.NewServer(httpadapter.ServerConfig{
		Addr:        cfg.HTTPAddr,
		ServiceName: cfg.ServiceName,
		MetricsPath: cfg.MetricsPath,
		Auth: &httpadapter.AuthConfig{
			JWTSecret:  cfg.JWTSecret,
			ServiceCAs: serviceCAs,
			ServiceCNs: cfg.ServiceCNs(),
		},
	}, handler, authHandler, appMetrics)

	tlsConfig, err := tlsutil.LoadServerTLSConfig(cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("load tls config: %w", err)
	}

	server.TLSConfig = tlsConfig
	return server, nil
}

func buildDependencies(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
) (app.CartRepository, app.OrderRepository, app.EventPublisher, *health.Checker, *app.AuthService, func(), error) {
	if cfg.UseMemory {
		logger.Info("using in-memory repositories")
		return cartmemory.NewCartRepository(), ordermemory.NewOrderRepository(), app.NewNoopEventPublisher(), health.NewChecker(nil), nil, func() {}, nil
	}

	if cfg.DatabaseURL == "" {
		return nil, nil, nil, nil, nil, nil, errConfig("DATABASE_URL is required when USE_MEMORY=false")
	}

	pool, err := newPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	db := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(db); err != nil {
		closePostgres(db, pool)
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("migrate postgres: %w", err)
	}

	redisClient := goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		closePostgres(db, pool)
		_ = redisClient.Close()
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("connect redis: %w", err)
	}

	natsConn, err := connectNATS(cfg)
	if err != nil {
		closePostgres(db, pool)
		_ = redisClient.Close()
		return nil, nil, nil, nil, nil, nil, err
	}

	orderRepo := orderpostgres.NewOrderRepository(pool)
	cartRepo := cartredis.NewCartRepository(redisClient)
	userRepo := userpostgres.NewUserRepository(pool)
	authService := app.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTTTL)
	events := natsadapter.NewPublisher(natsConn, natsadapter.Subjects{
		OrderCreated:       cfg.OrderCreatedSubject,
		ReserveItems:       cfg.ReserveItemsSubject,
		ConfirmOrder:       cfg.ConfirmOrderSubject,
		ReleaseReservation: cfg.ReleaseReservationSubject,
		OrderFinalized:     cfg.OrderFinalizedSubject,
		OrderCancelled:     cfg.OrderCancelledSubject,
	})

	checks := map[string]health.CheckFunc{
		"postgres": orderRepo.Ping,
		"redis":    cartRepo.Ping,
		"nats": func(context.Context) error {
			if !natsConn.IsConnected() {
				return errConfig("nats is not connected")
			}
			return nil
		},
	}

	cleanup := func() {
		natsConn.Close()
		_ = redisClient.Close()
		closePostgres(db, pool)
	}

	return cartRepo, orderRepo, events, health.NewChecker(checks), authService, cleanup, nil
}

func connectNATS(cfg *config.Config) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.ClientCert(cfg.NATSTLSCertFile, cfg.NATSTLSKeyFile),
		nats.RootCAs(cfg.NATSTLSCAFile),
	}

	conn, err := nats.Connect(cfg.NATSURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	return conn, nil
}

func newPostgresPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	poolConfig.ConnConfig.Tracer = otelpgx.NewTracer()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	return pool, nil
}

func startActiveOrdersRefresh(
	ctx context.Context,
	cfg *config.Config,
	appMetrics *metrics.Metrics,
	orderRepo app.OrderRepository,
	logger *slog.Logger,
) {
	refresh := func() {
		count, err := orderRepo.CountActiveOrders(ctx)
		if err != nil {
			logger.Warn("active orders refresh failed", slog.Any("error", err))
			return
		}

		appMetrics.SetActiveOrders(count)
	}

	refresh()

	interval := time.Duration(cfg.ActiveOrdersRefreshSec) * time.Second
	if interval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}

func closePostgres(db interface{ Close() error }, pool *pgxpool.Pool) {
	_ = db.Close()
	pool.Close()
}

type configError string

func (e configError) Error() string {
	return string(e)
}

func errConfig(message string) error {
	return configError(message)
}
