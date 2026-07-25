package testwire

import (
	"context"
	"fmt"
	"net"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/payment-service/adapters/eventpublisher"
	grpcadapter "github.com/trb1maker/microservices/internal/payment-service/adapters/grpc"
	pgadapter "github.com/trb1maker/microservices/internal/payment-service/adapters/postgres"
	"github.com/trb1maker/microservices/internal/payment-service/app"
	"github.com/trb1maker/microservices/internal/payment-service/migrations"

	paymentpb "github.com/trb1maker/microservices/internal/platform/proto/payment"

	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type Subjects struct {
	PaymentSucceeded string
	PaymentFailed    string
	RefundSucceeded  string
	RefundFailed     string
}

type GRPCServer struct {
	Addr   string
	server *grpc.Server
	lis    net.Listener
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

func StartInsecureGRPC(ctx context.Context, pool *pgxpool.Pool, nc *nats.Conn, subjects Subjects) (*GRPCServer, error) {
	accountRepo := pgadapter.NewAccountRepository(pool)
	txRepo := pgadapter.NewTransactionRepository(pool)
	eventPub := eventpublisher.NewNATSEventPublisher(
		nc,
		subjects.PaymentSucceeded,
		subjects.PaymentFailed,
		subjects.RefundSucceeded,
		subjects.RefundFailed,
	)
	svc := app.NewPaymentService(accountRepo, txRepo, eventPub)

	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
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

	return &GRPCServer{Addr: lis.Addr().String(), server: srv, lis: lis}, nil
}

func (s *GRPCServer) Close() {
	if s == nil || s.server == nil {
		return
	}
	s.server.GracefulStop()
	if s.lis != nil {
		_ = s.lis.Close()
	}
}
