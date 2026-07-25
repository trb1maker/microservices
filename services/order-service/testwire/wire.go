package testwire

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"

	cartredis "github.com/trb1maker/microservices/services/order-service/internal/adapters/cart_repository/redis"
	natsconsumer "github.com/trb1maker/microservices/services/order-service/internal/adapters/event_consumer/nats"
	natsadapter "github.com/trb1maker/microservices/services/order-service/internal/adapters/event_publisher/nats"
	httpadapter "github.com/trb1maker/microservices/services/order-service/internal/adapters/http"
	orderpostgres "github.com/trb1maker/microservices/services/order-service/internal/adapters/order_repository/postgres"
	paymentgrpc "github.com/trb1maker/microservices/services/order-service/internal/adapters/payment/grpc"
	userpostgres "github.com/trb1maker/microservices/services/order-service/internal/adapters/user_repository/postgres"
	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"
	"github.com/trb1maker/microservices/services/order-service/migrations"

	"github.com/trb1maker/microservices/pkg/health"
)

var errNatsNotConnected = errors.New("nats is not connected")

type Subjects struct {
	OrderCreated       string
	ReserveItems       string
	ConfirmOrder       string
	ReleaseReservation string
	OrderFinalized     string
	OrderCancelled     string
	ItemsReserved      string
	ReservationFailed  string
	OrderConfirmed     string
}

type Config struct {
	OrdersConn  string
	RedisClient *goredis.Client
	NatsConn    *nats.Conn
	PaymentAddr string
	JWTSecret   string
	Subjects    Subjects
}

type Stack struct {
	Server        *httptest.Server
	Consumer      *natsconsumer.Consumer
	PaymentClient *paymentgrpc.PaymentClient
	OrdersPool    *pgxpool.Pool
	cartRepo      *cartredis.CartRepository
}

func SetupOrdersDatabase(ctx context.Context, connStr string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("connect orders postgres: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(db); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate orders postgres: %w", err)
	}
	if err := db.Close(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("close migration db: %w", err)
	}
	return pool, nil
}

func SetupStack(ctx context.Context, cfg Config) (*Stack, error) {
	ordersPool, err := SetupOrdersDatabase(ctx, cfg.OrdersConn)
	if err != nil {
		return nil, err
	}

	orderRepo := orderpostgres.NewOrderRepository(ordersPool)
	cartRepo := cartredis.NewCartRepository(cfg.RedisClient)
	userRepo := userpostgres.NewUserRepository(ordersPool)
	authService := app.NewAuthService(userRepo, cfg.JWTSecret, time.Hour)
	events := natsadapter.NewPublisher(cfg.NatsConn, natsadapter.Subjects{
		OrderCreated:       cfg.Subjects.OrderCreated,
		ReserveItems:       cfg.Subjects.ReserveItems,
		ConfirmOrder:       cfg.Subjects.ConfirmOrder,
		ReleaseReservation: cfg.Subjects.ReleaseReservation,
		OrderFinalized:     cfg.Subjects.OrderFinalized,
		OrderCancelled:     cfg.Subjects.OrderCancelled,
	})

	paymentClient, err := paymentgrpc.NewPaymentClient(ctx, paymentgrpc.ClientConfig{
		Addr:     cfg.PaymentAddr,
		Insecure: true,
	})
	if err != nil {
		ordersPool.Close()
		return nil, fmt.Errorf("create payment client: %w", err)
	}

	cartService := app.NewCartService(cartRepo, events)
	orderService := app.NewOrderService(cartRepo, orderRepo, events, paymentClient, app.NewNoopOrderMetrics())
	consumer := natsconsumer.NewConsumer(cfg.NatsConn, natsconsumer.Subjects{
		ItemsReserved:     cfg.Subjects.ItemsReserved,
		ReservationFailed: cfg.Subjects.ReservationFailed,
		OrderConfirmed:    cfg.Subjects.OrderConfirmed,
	}, cartService, orderService)
	if err := consumer.Start(); err != nil {
		_ = paymentClient.Close()
		ordersPool.Close()
		return nil, fmt.Errorf("start order consumer: %w", err)
	}

	checks := map[string]health.CheckFunc{
		"postgres": orderRepo.Ping,
		"redis":    cartRepo.Ping,
		"nats": func(context.Context) error {
			if !cfg.NatsConn.IsConnected() {
				return errNatsNotConnected
			}
			return nil
		},
	}
	handler := httpadapter.NewHandler(cartService, orderService, health.NewChecker(checks), nil)
	server := httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{
		Addr: ":8080",
		Auth: &httpadapter.AuthConfig{JWTSecret: cfg.JWTSecret},
	}, handler, httpadapter.NewAppAuthAdapter(authService), nil).Handler)

	return &Stack{
		Server:        server,
		Consumer:      consumer,
		PaymentClient: paymentClient,
		OrdersPool:    ordersPool,
		cartRepo:      cartRepo,
	}, nil
}

func (s *Stack) CartAllItemsReserved(ctx context.Context, userID string) bool {
	if s == nil || s.cartRepo == nil {
		return false
	}
	id, err := uuid.Parse(userID)
	if err != nil {
		return false
	}
	cart, err := s.cartRepo.Get(ctx, domain.UserID(id))
	if err != nil || cart == nil {
		return false
	}
	return cart.AllItemsReserved()
}

func (s *Stack) Close() {
	if s == nil {
		return
	}
	if s.Server != nil {
		s.Server.Close()
	}
	if s.Consumer != nil {
		s.Consumer.Close()
	}
	if s.PaymentClient != nil {
		_ = s.PaymentClient.Close()
	}
	if s.OrdersPool != nil {
		s.OrdersPool.Close()
	}
}
