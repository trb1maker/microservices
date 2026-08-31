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

	cartmemory "github.com/trb1maker/microservices/internal/order-service/adapters/cart_repository/memory"
	cartredis "github.com/trb1maker/microservices/internal/order-service/adapters/cart_repository/redis"
	checkoutpostgres "github.com/trb1maker/microservices/internal/order-service/adapters/checkout_writer/postgres"
	natsconsumer "github.com/trb1maker/microservices/internal/order-service/adapters/event_consumer/nats"
	natsadapter "github.com/trb1maker/microservices/internal/order-service/adapters/event_publisher/nats"
	grpcadapter "github.com/trb1maker/microservices/internal/order-service/adapters/grpc"
	httpadapter "github.com/trb1maker/microservices/internal/order-service/adapters/http"
	ordermemory "github.com/trb1maker/microservices/internal/order-service/adapters/order_repository/memory"
	orderpostgres "github.com/trb1maker/microservices/internal/order-service/adapters/order_repository/postgres"
	paymentgrpc "github.com/trb1maker/microservices/internal/order-service/adapters/payment/grpc"
	userpostgres "github.com/trb1maker/microservices/internal/order-service/adapters/user_repository/postgres"
	"github.com/trb1maker/microservices/internal/order-service/app"
	"github.com/trb1maker/microservices/internal/order-service/config"
	"github.com/trb1maker/microservices/internal/order-service/migrations"
	"github.com/trb1maker/microservices/internal/platform/outbox"
	outboxnats "github.com/trb1maker/microservices/internal/platform/outbox/natspub"
	outboxpg "github.com/trb1maker/microservices/internal/platform/outbox/pgstore"

	"github.com/trb1maker/microservices/internal/platform/health"
	"github.com/trb1maker/microservices/internal/platform/logging"
	"github.com/trb1maker/microservices/internal/platform/metrics"
	pkgmiddleware "github.com/trb1maker/microservices/internal/platform/middleware"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	pkgotel "github.com/trb1maker/microservices/internal/platform/otel"
	"github.com/trb1maker/microservices/internal/platform/tlsutil"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"

	_ "github.com/trb1maker/microservices/internal/order-service/docs/swagger"
)

// @title           Order Service API
// @version         1.0
// @description     REST API for cart, checkout, and order management.
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("application failed", slog.Any("error", err))
		os.Exit(1)
	}
}

//nolint:funlen // Composition root intentionally keeps lifecycle together.
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

	deps, cleanup, err := buildDependencies(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("build dependencies: %w", err)
	}
	defer cleanup()

	startActiveOrdersRefresh(ctx, cfg, appMetrics, deps.orderRepo, logger)

	server, err := newHTTPServer(cfg, deps, appMetrics, deps.redisClient)
	if err != nil {
		return err
	}

	var grpcServer *grpcadapter.Server
	if !cfg.UseMemory {
		grpcServer, err = grpcadapter.NewServer(grpcadapter.ServerConfig{
			Addr:         cfg.GRPCAddr,
			CertFile:     cfg.TLSCertFile,
			KeyFile:      cfg.TLSKeyFile,
			ClientCAFile: cfg.TLSClientCAFile,
		}, deps.orderService, deps.statusHub)
		if err != nil {
			return fmt.Errorf("create grpc server: %w", err)
		}

		go func() {
			logger.Info("grpc server started", slog.String("addr", cfg.GRPCAddr))
			if serveErr := grpcServer.Serve(); serveErr != nil {
				logger.Error("grpc server failed", slog.Any("error", serveErr))
			}
		}()
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

	if grpcServer != nil {
		grpcServer.GracefulStop()
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	if err := shutdownTracer(shutdownCtx); err != nil {
		return fmt.Errorf("tracer shutdown: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

type dependencies struct {
	cartRepo      app.CartRepository
	orderRepo     app.OrderRepository
	events        app.EventPublisher
	readiness     *health.Checker
	authService   *app.AuthService
	cartService   *app.CartService
	orderService  *app.OrderService
	statusHub     *grpcadapter.StatusHub
	natsConsumer  *natsconsumer.Consumer
	paymentClient *paymentgrpc.PaymentClient
	redisClient   *goredis.Client
	relayCancel   context.CancelFunc
}

func newHTTPServer(cfg *config.Config, deps *dependencies, appMetrics *metrics.Metrics, redisClient *goredis.Client) (*http.Server, error) {
	var remoteChecker httpadapter.RemoteHealthChecker
	if !cfg.UseMemory && deps.paymentClient != nil {
		remoteChecker = httpadapter.NewHTTPRemoteChecker(cfg.StoreHealthURL, deps.paymentClient)
	}
	dashboard, err := httpadapter.NewStatusDashboard(deps.readiness, remoteChecker)
	if err != nil {
		return nil, fmt.Errorf("create status dashboard: %w", err)
	}

	handler := httpadapter.NewHandler(deps.cartService, deps.orderService, deps.readiness, dashboard)

	serviceCAs, err := tlsutil.LoadClientCAPool(cfg.TLSClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("load client ca: %w", err)
	}

	var authHandler httpadapter.AuthService
	if deps.authService != nil {
		authHandler = httpadapter.NewAppAuthAdapter(deps.authService)
	}

	var rateLimit *pkgmiddleware.RateLimitConfig
	if cfg.RateLimitEnabled && redisClient != nil {
		metricsPath := cfg.MetricsPath
		if metricsPath == "" {
			metricsPath = "/metrics"
		}

		rateLimit = &pkgmiddleware.RateLimitConfig{
			Client:  redisClient,
			Limit:   cfg.RateLimitRequests,
			Window:  cfg.RateLimitWindow,
			Skip:    pkgmiddleware.SkipRateLimitPaths(metricsPath),
			OnLimit: appMetrics.RecordRateLimitExceeded,
		}
	}

	server := httpadapter.NewServer(httpadapter.ServerConfig{
		Addr:        cfg.HTTPAddr,
		ServiceName: cfg.ServiceName,
		MetricsPath: cfg.MetricsPath,
		RateLimit:   rateLimit,
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

//nolint:funlen // Dependency construction includes cleanup for every resource.
func buildDependencies(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
) (*dependencies, func(), error) {
	if cfg.UseMemory {
		logger.Info("using in-memory repositories")
		cartRepo := cartmemory.NewCartRepository()
		orderRepo := ordermemory.NewOrderRepository()
		events := app.NewNoopEventPublisher()
		statusHub := grpcadapter.NewStatusHub()
		cartService := app.NewCartService(cartRepo, events)
		orderService := app.NewOrderService(cartRepo, orderRepo, events, statusHub, app.NewNoopOrderMetrics())
		return &dependencies{
			cartRepo:     cartRepo,
			orderRepo:    orderRepo,
			events:       events,
			readiness:    health.NewChecker(nil),
			cartService:  cartService,
			orderService: orderService,
			statusHub:    statusHub,
		}, func() {}, nil
	}

	if cfg.DatabaseURL == "" {
		return nil, nil, errConfig("DATABASE_URL is required when USE_MEMORY=false")
	}

	pool, err := newPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}

	db := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(db); err != nil {
		closePostgres(db, pool)
		return nil, nil, fmt.Errorf("migrate postgres: %w", err)
	}

	redisClient := goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		closePostgres(db, pool)
		_ = redisClient.Close()
		return nil, nil, fmt.Errorf("connect redis: %w", err)
	}

	natsConn, err := connectNATS(cfg)
	if err != nil {
		closePostgres(db, pool)
		_ = redisClient.Close()
		return nil, nil, err
	}

	natsClient, err := natsx.New(ctx, natsConn)
	if err != nil {
		natsConn.Close()
		closePostgres(db, pool)
		_ = redisClient.Close()
		return nil, nil, fmt.Errorf("init jetstream: %w", err)
	}

	orderRepo := orderpostgres.NewOrderRepository(pool)
	cartRepo := cartredis.NewCartRepository(redisClient, cfg.CartTTL)
	userRepo := userpostgres.NewUserRepository(pool)
	authService := app.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTTTL)
	events := natsadapter.NewPublisher(natsClient, natsadapter.Subjects{
		OrderCreated:       cfg.OrderCreatedSubject,
		ReserveItems:       cfg.ReserveItemsSubject,
		ConfirmOrder:       cfg.ConfirmOrderSubject,
		ReleaseReservation: cfg.ReleaseReservationSubject,
		OrderFinalized:     cfg.OrderFinalizedSubject,
		OrderCancelled:     cfg.OrderCancelledSubject,
	})

	statusHub := grpcadapter.NewStatusHub()

	paymentClient, err := paymentgrpc.NewPaymentClient(ctx, paymentgrpc.ClientConfig{
		Addr:       cfg.PaymentGRPCAddr,
		CertFile:   cfg.NATSTLSCertFile,
		KeyFile:    cfg.NATSTLSKeyFile,
		CAFile:     cfg.NATSTLSCAFile,
		ServerName: "payment-service",
	})
	if err != nil {
		natsConn.Close()
		closePostgres(db, pool)
		_ = redisClient.Close()
		return nil, nil, fmt.Errorf("connect payment service: %w", err)
	}

	cartService := app.NewCartService(cartRepo, events)
	checkoutWriter := checkoutpostgres.NewWriter(pool)
	orderService := app.NewOrderService(
		cartRepo,
		orderRepo,
		events,
		paymentClient,
		statusHub,
		app.NewNoopOrderMetrics(),
		checkoutWriter,
		app.OrderCreatedSubject(cfg.OrderCreatedSubject),
	)

	relay := outbox.NewRelay(outboxpg.New(pool), outboxnats.New(natsClient), outbox.RelayConfig{
		PollInterval: cfg.OutboxPollInterval,
		BatchSize:    cfg.OutboxBatchSize,
	})
	relayCtx, relayCancel := context.WithCancel(ctx)
	go func() {
		if err := relay.Run(relayCtx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("outbox relay stopped", slog.Any("error", err))
		}
	}()

	consumer := natsconsumer.NewConsumer(natsClient, natsconsumer.Subjects{
		ItemsReserved:     cfg.ItemsReservedSubject,
		ReservationFailed: cfg.ReservationFailedSubject,
		OrderConfirmed:    cfg.OrderConfirmedSubject,
	}, cartService, orderService)
	if err := consumer.Start(ctx); err != nil {
		relayCancel()
		_ = paymentClient.Close()
		natsConn.Close()
		closePostgres(db, pool)
		_ = redisClient.Close()
		return nil, nil, fmt.Errorf("start nats consumer: %w", err)
	}

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
		relayCancel()
		consumer.Close()
		_ = paymentClient.Close()
		natsConn.Close()
		_ = redisClient.Close()
		closePostgres(db, pool)
	}

	return &dependencies{
		cartRepo:      cartRepo,
		orderRepo:     orderRepo,
		events:        events,
		readiness:     health.NewChecker(checks),
		authService:   authService,
		cartService:   cartService,
		orderService:  orderService,
		statusHub:     statusHub,
		natsConsumer:  consumer,
		paymentClient: paymentClient,
		redisClient:   redisClient,
		relayCancel:   relayCancel,
	}, cleanup, nil
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
