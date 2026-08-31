package testwire

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/trb1maker/microservices/internal/payment-service/adapters/eventpublisher"
	grpcadapter "github.com/trb1maker/microservices/internal/payment-service/adapters/grpc"
	pgadapter "github.com/trb1maker/microservices/internal/payment-service/adapters/postgres"
	"github.com/trb1maker/microservices/internal/payment-service/app"
	"github.com/trb1maker/microservices/internal/payment-service/migrations"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	"github.com/trb1maker/microservices/internal/platform/outbox"
	outboxnats "github.com/trb1maker/microservices/internal/platform/outbox/natspub"
	outboxpg "github.com/trb1maker/microservices/internal/platform/outbox/pgstore"

	paymentpb "github.com/trb1maker/microservices/internal/platform/proto/payment"

	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const (
	testOutboxPollInterval = 50 * time.Millisecond
	testOutboxBatchSize    = 50
)

type Subjects struct {
	PaymentSucceeded string
	PaymentFailed    string
	RefundSucceeded  string
	RefundFailed     string
}

type GRPCServer struct {
	Addr        string
	server      *grpc.Server
	lis         net.Listener
	relayCancel context.CancelFunc
}

func SetupDatabase(ctx context.Context, connStr string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(db); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate postgres: %w", err)
	}
	if err := db.Close(); err != nil {
		return nil, fmt.Errorf("close migration db: %w", err)
	}
	return pool, nil
}

func SeedAccount(ctx context.Context, pool *pgxpool.Pool, userID string, balance int64) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO accounts (user_id, balance, version)
		VALUES ($1, $2, 1)
		ON CONFLICT (user_id) DO UPDATE SET balance = EXCLUDED.balance
	`, userID, balance)
	if err != nil {
		return fmt.Errorf("seed account: %w", err)
	}
	return nil
}

func StartInsecureGRPC(ctx context.Context, pool *pgxpool.Pool, client *natsx.Client, subjects Subjects) (*GRPCServer, error) {
	accountRepo := pgadapter.NewAccountRepository(pool)
	txRepo := pgadapter.NewTransactionRepository(pool)
	eventPub := eventpublisher.NewOutboxPublisher(
		pool,
		subjects.PaymentSucceeded,
		subjects.PaymentFailed,
		subjects.RefundSucceeded,
		subjects.RefundFailed,
	)
	svc := app.NewPaymentService(accountRepo, txRepo, eventPub)
	relayCtx, relayCancel := context.WithCancel(ctx)
	relay := outbox.NewRelay(outboxpg.New(pool), outboxnats.New(client), outbox.RelayConfig{
		PollInterval: testOutboxPollInterval,
		BatchSize:    testOutboxBatchSize,
	})
	go func() { _ = relay.Run(relayCtx) }()

	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		relayCancel()
		return nil, fmt.Errorf("listen grpc: %w", err)
	}

	srv := grpc.NewServer()
	paymentpb.RegisterPaymentServiceServer(srv, grpcadapter.NewPaymentServer(svc))
	healthServer := grpchealth.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	go func() {
		_ = srv.Serve(lis)
	}()

	return &GRPCServer{Addr: lis.Addr().String(), server: srv, lis: lis, relayCancel: relayCancel}, nil
}

func (s *GRPCServer) Close() {
	if s == nil {
		return
	}
	if s.relayCancel != nil {
		s.relayCancel()
	}
	if s.server == nil {
		return
	}
	s.server.GracefulStop()
	if s.lis != nil {
		_ = s.lis.Close()
	}
}
