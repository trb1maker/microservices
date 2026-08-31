package testwire

import (
	"context"
	"fmt"
	"net/http/httptest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	docpostgres "github.com/trb1maker/microservices/internal/analytics-service/adapters/document_repository/postgres"
	natsconsumer "github.com/trb1maker/microservices/internal/analytics-service/adapters/event_consumer/nats"
	analyticshttp "github.com/trb1maker/microservices/internal/analytics-service/adapters/http"
	minioadapter "github.com/trb1maker/microservices/internal/analytics-service/adapters/receipt_storage/minio"
	summarypostgres "github.com/trb1maker/microservices/internal/analytics-service/adapters/summary_repository/postgres"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
	"github.com/trb1maker/microservices/internal/analytics-service/migrations"
	"github.com/trb1maker/microservices/internal/platform/health"
	"github.com/trb1maker/microservices/internal/platform/metrics"
	"github.com/trb1maker/microservices/internal/platform/natsx"
)

type MinIOConfig struct {
	Endpoint       string
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	Bucket         string
}

type Consumer struct {
	consumer   *natsconsumer.Consumer
	storage    *minioadapter.Storage
	summary    *summarypostgres.SummaryRepository
	documents  *docpostgres.DocumentRepository
	svc        *app.AnalyticsService
	pool       *pgxpool.Pool
	httpServer *httptest.Server
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
	client *natsx.Client,
	pool *pgxpool.Pool,
	minioCfg MinIOConfig,
	orderFinalizedSubject string,
) (*Consumer, error) {
	return StartConsumerWithHTTP(ctx, client, pool, minioCfg, orderFinalizedSubject, "")
}

func StartConsumerWithHTTP(
	ctx context.Context,
	client *natsx.Client,
	pool *pgxpool.Pool,
	minioCfg MinIOConfig,
	orderFinalizedSubject string,
	jwtSecret string,
) (*Consumer, error) {
	storage, err := minioadapter.NewStorage(minioadapter.Config{
		Endpoint:       minioCfg.Endpoint,
		PublicEndpoint: minioCfg.PublicEndpoint,
		AccessKey:      minioCfg.AccessKey,
		SecretKey:      minioCfg.SecretKey,
		Bucket:         minioCfg.Bucket,
		UseSSL:         false,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio storage: %w", err)
	}

	summaryRepo := summarypostgres.NewSummaryRepository(pool)
	documentRepo := docpostgres.NewDocumentRepository(pool)
	svc := app.NewAnalyticsService(storage, summaryRepo, documentRepo, app.DefaultReceiptURLTTL)
	consumer := natsconsumer.NewConsumer(client, orderFinalizedSubject, svc)
	if err := consumer.Start(context.WithoutCancel(ctx)); err != nil {
		return nil, fmt.Errorf("start analytics consumer: %w", err)
	}

	c := &Consumer{
		consumer:  consumer,
		storage:   storage,
		summary:   summaryRepo,
		documents: documentRepo,
		svc:       svc,
		pool:      pool,
	}

	if jwtSecret != "" {
		readiness := health.NewChecker(map[string]health.CheckFunc{"postgres": summaryRepo.Ping})
		handler := analyticshttp.NewHandler(svc, readinessAdapter{checker: readiness})
		server := analyticshttp.NewServer(analyticshttp.ServerConfig{
			Addr:        ":0",
			ServiceName: "analytics-service",
			MetricsPath: "/metrics",
			JWTSecret:   jwtSecret,
		}, handler, metrics.New("/metrics"))
		c.httpServer = httptest.NewServer(server.Handler)
	}

	_ = ctx
	return c, nil
}

type readinessAdapter struct {
	checker *health.Checker
}

func (r readinessAdapter) Check(ctx context.Context) (bool, map[string]string) {
	return r.checker.Check(ctx)
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

func (c *Consumer) HTTPURL() string {
	if c == nil || c.httpServer == nil {
		return ""
	}
	return c.httpServer.URL
}

func (c *Consumer) Close() {
	if c == nil {
		return
	}
	if c.httpServer != nil {
		c.httpServer.Close()
	}
	if c.consumer == nil {
		return
	}
	c.consumer.Close()
}
