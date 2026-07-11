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
	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/internal/config"
	"github.com/trb1maker/microservices/services/order-service/migrations"

	"github.com/trb1maker/microservices/pkg/health"
	"github.com/trb1maker/microservices/pkg/logging"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"
)

const shutdownTimeout = 10 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	logger, err := logging.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		slog.Error("failed to init logger", slog.Any("error", err))
		os.Exit(1)
	}

	slog.SetDefault(logger)

	ctx := context.Background()

	cartRepo, orderRepo, events, readiness, cleanup, err := buildDependencies(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to build dependencies", slog.Any("error", err))
		os.Exit(1)
	}
	defer cleanup()

	cartService := app.NewCartService(cartRepo)
	orderService := app.NewOrderService(cartRepo, orderRepo, events)
	handler := httpadapter.NewHandler(cartService, orderService, readiness)
	server := httpadapter.NewServer(cfg.HTTPAddr, handler)

	go func() {
		logger.Info("server started", slog.String("addr", cfg.HTTPAddr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", slog.Any("error", err))
	}

	logger.Info("server stopped")
}

func buildDependencies(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
) (app.CartRepository, app.OrderRepository, app.EventPublisher, *health.Checker, func(), error) {
	if cfg.UseMemory {
		logger.Info("using in-memory repositories")
		return cartmemory.NewCartRepository(), ordermemory.NewOrderRepository(), app.NewNoopEventPublisher(), health.NewChecker(nil), func() {}, nil
	}

	if cfg.DatabaseURL == "" {
		return nil, nil, nil, nil, nil, errConfig("DATABASE_URL is required when USE_MEMORY=false")
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("connect postgres: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(db); err != nil {
		closePostgres(db, pool)
		return nil, nil, nil, nil, nil, fmt.Errorf("migrate postgres: %w", err)
	}

	redisClient := goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		closePostgres(db, pool)
		_ = redisClient.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("connect redis: %w", err)
	}

	natsConn, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		closePostgres(db, pool)
		_ = redisClient.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("connect nats: %w", err)
	}

	orderRepo := orderpostgres.NewOrderRepository(pool)
	cartRepo := cartredis.NewCartRepository(redisClient)
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

	return cartRepo, orderRepo, events, health.NewChecker(checks), cleanup, nil
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
