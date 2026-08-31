//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	docpostgres "github.com/trb1maker/microservices/internal/analytics-service/adapters/document_repository/postgres"
	natsconsumer "github.com/trb1maker/microservices/internal/analytics-service/adapters/event_consumer/nats"
	analyticshttp "github.com/trb1maker/microservices/internal/analytics-service/adapters/http"
	minioadapter "github.com/trb1maker/microservices/internal/analytics-service/adapters/receipt_storage/minio"
	summarypostgres "github.com/trb1maker/microservices/internal/analytics-service/adapters/summary_repository/postgres"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
	"github.com/trb1maker/microservices/internal/analytics-service/migrations"
	"github.com/trb1maker/microservices/internal/platform/auth"
	"github.com/trb1maker/microservices/internal/platform/health"
	"github.com/trb1maker/microservices/internal/platform/metrics"
	"github.com/trb1maker/microservices/tests/internal/natstest"
)

const (
	orderFinalizedSubject = "orders.finalized"
	testJWTSecret         = "test-jwt-secret-minimum-32-characters"
)

var (
	minioAccessKey = envOr("MINIO_ACCESS_KEY", "minioadmin")
	minioSecretKey = envOr("MINIO_SECRET_KEY", "minioadmin")
	minioBucket    = envOr("MINIO_BUCKET", "receipts")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestIntegration_OrderFinalizedFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	consumerCtx := context.WithoutCancel(ctx)

	pool := startAnalyticsPostgres(t, ctx)
	minioEndpoint, minioClient := startMinIO(t, ctx)

	receiptStorage, err := minioadapter.NewStorage(minioadapter.Config{
		Endpoint:  minioEndpoint,
		AccessKey: minioAccessKey,
		SecretKey: minioSecretKey,
		Bucket:    minioBucket,
		UseSSL:    false,
	})
	require.NoError(t, err)

	summaryRepo := summarypostgres.NewSummaryRepository(pool)
	documentRepo := docpostgres.NewDocumentRepository(pool)
	analyticsSvc := app.NewAnalyticsService(receiptStorage, summaryRepo, documentRepo, 15*time.Minute)

	client := natstest.NewClient(t, mustNATSURL(t, ctx))
	t.Cleanup(client.Conn().Close)

	analyticsConsumer := natsconsumer.NewConsumer(client, orderFinalizedSubject, analyticsSvc)
	require.NoError(t, analyticsConsumer.Start(consumerCtx))
	t.Cleanup(analyticsConsumer.Close)

	orderID := uuid.NewV7().String()
	userID := uuid.NewV7().String()
	finalizedAt := time.Now().UTC().Format(time.RFC3339)
	event := app.OrderFinalized{
		OrderID:         orderID,
		UserID:          userID,
		TotalAmount:     2500,
		Status:          "CONFIRMED",
		FinalizedAt:     finalizedAt,
		DeliveryAddress: "Moscow Red Square 1",
	}

	payload, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, client.Publish(ctx, orderFinalizedSubject, payload))
	require.NoError(t, client.Publish(ctx, orderFinalizedSubject, payload))

	require.Eventually(t, func() bool {
		exists, err := receiptStorage.Exists(ctx, orderID)
		return err == nil && exists
	}, 10*time.Second, 100*time.Millisecond)

	object, err := minioClient.GetObject(ctx, minioBucket, "receipts/"+orderID+".json", minio.GetObjectOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = object.Close() })

	var receipt app.Receipt
	require.NoError(t, json.NewDecoder(object).Decode(&receipt))
	assert.Equal(t, orderID, receipt.OrderID)
	assert.Equal(t, userID, receipt.UserID)
	assert.Equal(t, int64(2500), receipt.TotalAmount)
	assert.Equal(t, "CONFIRMED", receipt.Status)
	assert.Equal(t, "Moscow Red Square 1", receipt.DeliveryAddress)

	var totalOrders int
	var totalRevenue int64
	summaryDate := time.Now().UTC().Truncate(24 * time.Hour)
	err = pool.QueryRow(ctx, `
		SELECT total_orders, total_revenue
		FROM daily_summary
		WHERE date = $1
	`, summaryDate).Scan(&totalOrders, &totalRevenue)
	require.NoError(t, err)
	assert.Equal(t, 1, totalOrders)
	assert.Equal(t, int64(2500), totalRevenue)

	var processedCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM processed_orders WHERE order_id = $1`, orderID).Scan(&processedCount)
	require.NoError(t, err)
	assert.Equal(t, 1, processedCount)

	var docCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM receipt_documents WHERE order_id = $1`, orderID).Scan(&docCount)
	require.NoError(t, err)
	assert.Equal(t, 1, docCount)

	readiness := health.NewChecker(map[string]health.CheckFunc{"postgres": summaryRepo.Ping})
	handler := analyticshttp.NewHandler(analyticsSvc, readinessAdapter{checker: readiness})
	server := analyticshttp.NewServer(analyticshttp.ServerConfig{
		Addr:        ":0",
		ServiceName: "analytics-service",
		MetricsPath: "/metrics",
		JWTSecret:   testJWTSecret,
	}, handler, metrics.New("/metrics"))
	httpServer := httptest.NewServer(server.Handler)
	t.Cleanup(httpServer.Close)

	token, err := auth.IssueToken(testJWTSecret, uuid.MustParse(userID), time.Hour)
	require.NoError(t, err)

	receiptReq, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/receipts/"+orderID, nil)
	require.NoError(t, err)
	receiptReq.Header.Set("Authorization", "Bearer "+token)
	receiptResp, err := http.DefaultClient.Do(receiptReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = receiptResp.Body.Close() })
	require.Equal(t, http.StatusOK, receiptResp.StatusCode)

	var receiptURL struct {
		URL       string `json:"url"`
		ExpiresIn int64  `json:"expires_in"`
	}
	require.NoError(t, json.NewDecoder(receiptResp.Body).Decode(&receiptURL))
	assert.Contains(t, receiptURL.URL, orderID)
	assert.Equal(t, int64(900), receiptURL.ExpiresIn)

	searchReq, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/receipts/search?q=Moscow", nil)
	require.NoError(t, err)
	searchReq.Header.Set("Authorization", "Bearer "+token)
	searchResp, err := http.DefaultClient.Do(searchReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = searchResp.Body.Close() })
	require.Equal(t, http.StatusOK, searchResp.StatusCode)

	var searchBody struct {
		Results []app.SearchResult `json:"results"`
	}
	require.NoError(t, json.NewDecoder(searchResp.Body).Decode(&searchBody))
	require.Len(t, searchBody.Results, 1)
	assert.Equal(t, orderID, searchBody.Results[0].OrderID)
}

type readinessAdapter struct {
	checker *health.Checker
}

func (r readinessAdapter) Check(ctx context.Context) (bool, map[string]string) {
	return r.checker.Check(ctx)
}

func startAnalyticsPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	pgContainer, err := postgres.Run(
		ctx,
		"postgres:18.4-alpine",
		postgres.WithDatabase("analytics"),
		postgres.WithUsername("analytics"),
		postgres.WithPassword("analytics"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(context.Background()) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	db := stdlib.OpenDBFromPool(pool)
	require.NoError(t, migrations.Up(db))
	require.NoError(t, db.Close())
	return pool
}

func mustNATSURL(t *testing.T, ctx context.Context) string {
	t.Helper()
	natsContainer, err := tcnats.Run(ctx, natstest.Image, natstest.ContainerOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })
	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)
	return natsURL
}

func startMinIO(t *testing.T, ctx context.Context) (string, *minio.Client) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:RELEASE.2025-04-22T22-12-26Z",
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     minioAccessKey,
			"MINIO_ROOT_PASSWORD": minioSecretKey,
		},
		Cmd:        []string{"server", "/data"},
		WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "9000/tcp")
	require.NoError(t, err)

	endpoint := fmt.Sprintf("%s:%s", host, port.Port())
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
		Secure: false,
	})
	require.NoError(t, err)
	require.NoError(t, client.MakeBucket(ctx, minioBucket, minio.MakeBucketOptions{}))

	return endpoint, client
}
