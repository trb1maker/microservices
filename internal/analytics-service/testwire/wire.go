package testwire

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	natsconsumer "github.com/trb1maker/microservices/internal/analytics-service/adapters/event_consumer/nats"
	minioadapter "github.com/trb1maker/microservices/internal/analytics-service/adapters/receipt_storage/minio"
	summarypostgres "github.com/trb1maker/microservices/internal/analytics-service/adapters/summary_repository/postgres"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
	"github.com/trb1maker/microservices/internal/analytics-service/migrations"
)

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
}

type Consumer struct {
	consumer *natsconsumer.Consumer
	storage  *minioadapter.Storage
	summary  *summarypostgres.SummaryRepository
	pool     *pgxpool.Pool
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

func StartConsumer(
	ctx context.Context,
	nc *nats.Conn,
	pool *pgxpool.Pool,
	minioCfg MinIOConfig,
	orderFinalizedSubject string,
) (*Consumer, error) {
	storage, err := minioadapter.NewStorage(minioadapter.Config{
		Endpoint:  minioCfg.Endpoint,
		AccessKey: minioCfg.AccessKey,
		SecretKey: minioCfg.SecretKey,
		Bucket:    minioCfg.Bucket,
		UseSSL:    false,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio storage: %w", err)
	}

	summaryRepo := summarypostgres.NewSummaryRepository(pool)
	svc := app.NewAnalyticsService(storage, summaryRepo)
	consumer := natsconsumer.NewConsumer(nc, orderFinalizedSubject, svc)
	if err := consumer.Start(); err != nil {
		return nil, fmt.Errorf("start analytics consumer: %w", err)
	}
	_ = ctx
	return &Consumer{
		consumer: consumer,
		storage:  storage,
		summary:  summaryRepo,
		pool:     pool,
	}, nil
}

func (c *Consumer) Storage() *minioadapter.Storage {
	if c == nil {
		return nil
	}
	return c.storage
}

func (c *Consumer) Pool() *pgxpool.Pool {
	if c == nil {
		return nil
	}
	return c.pool
}

func (c *Consumer) Close() {
	if c == nil || c.consumer == nil {
		return
	}
	c.consumer.Close()
}
