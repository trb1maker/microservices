// go:build integration

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	natspkg "github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	analyticstestwire "github.com/trb1maker/microservices/services/analytics-service/testwire"
	notificationtestwire "github.com/trb1maker/microservices/services/notification-service/testwire"
	ordertestwire "github.com/trb1maker/microservices/services/order-service/testwire"
	paymenttestwire "github.com/trb1maker/microservices/services/payment-service/testwire"
	storetestwire "github.com/trb1maker/microservices/services/store-service/testwire"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type envOptions struct {
	gateConfirm bool
}

type env struct {
	server        *httptest.Server
	orderStack    *ordertestwire.Stack
	paymentsPool  *pgxpool.Pool
	analyticsPool *pgxpool.Pool
	mongoDB       *mongo.Database
	minioClient   *minio.Client
	natsConn      *natspkg.Conn
	token         string

	storeWorker  *storetestwire.Worker
	notification *notificationtestwire.Consumer
	analytics    *analyticstestwire.Consumer
	paymentGRPC  *paymenttestwire.GRPCServer

	pgContainer    testcontainers.Container
	redisContainer testcontainers.Container
	natsContainer  testcontainers.Container
	mongoContainer testcontainers.Container
	minioContainer testcontainers.Container
}

func newEnv(t *testing.T, opts ...envOptions) *env {
	t.Helper()

	var options envOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()

	pgContainer, ordersConn, paymentsConn, analyticsConn := startPostgres(t, ctx)
	redisClient, redisContainer := startRedis(t, ctx)
	natsConn, natsContainer := startNATS(t, ctx)
	mongoDB, mongoContainer := startMongo(t, ctx)
	minioEndpoint, minioClient, minioContainer := startMinIO(t, ctx)

	paymentsPool, err := paymenttestwire.SetupDatabase(ctx, paymentsConn)
	require.NoError(t, err)
	t.Cleanup(paymentsPool.Close)
	require.NoError(t, paymenttestwire.SeedAccount(ctx, paymentsPool, demoUserID, 100_000))

	analyticsPool, err := analyticstestwire.SetupDatabase(ctx, analyticsConn)
	require.NoError(t, err)
	t.Cleanup(analyticsPool.Close)

	storeWorker, err := storetestwire.SetupStore(ctx, mongoDB, natsConn, storetestwire.Subjects{
		ReserveItems:        reserveItemsSubject,
		ConfirmOrder:        confirmOrderSubject,
		ReleaseReservation:  releaseReservationSubject,
		ItemsReserved:       itemsReservedSubject,
		ReservationFailed:   reservationFailedSubject,
		OrderConfirmed:      orderConfirmedSubject,
		ReservationReleased: reservationReleasedSubject,
	}, storetestwire.Options{GateConfirm: options.gateConfirm})
	require.NoError(t, err)

	paymentGRPC, err := paymenttestwire.StartInsecureGRPC(ctx, paymentsPool, natsConn, paymenttestwire.Subjects{
		PaymentSucceeded: paymentSucceededSubject,
		PaymentFailed:    paymentFailedSubject,
		RefundSucceeded:  refundSucceededSubject,
		RefundFailed:     refundFailedSubject,
	})
	require.NoError(t, err)

	notification, err := notificationtestwire.StartConsumer(natsConn, notificationtestwire.Subjects{
		OrderFinalized:   orderFinalizedSubject,
		OrderCancelled:   orderCancelledSubject,
		PaymentSucceeded: paymentSucceededSubject,
		RefundSucceeded:  refundSucceededSubject,
	})
	require.NoError(t, err)

	analytics, err := analyticstestwire.StartConsumer(ctx, natsConn, analyticsPool, analyticstestwire.MinIOConfig{
		Endpoint:  minioEndpoint,
		AccessKey: minioAccessKey,
		SecretKey: minioSecretKey,
		Bucket:    minioBucket,
	}, orderFinalizedSubject)
	require.NoError(t, err)

	orderStack, err := ordertestwire.SetupStack(ctx, ordertestwire.Config{
		OrdersConn:  ordersConn,
		RedisClient: redisClient,
		NatsConn:    natsConn,
		PaymentAddr: paymentGRPC.Addr,
		JWTSecret:   testJWTSecret,
		Subjects: ordertestwire.Subjects{
			OrderCreated:       orderCreatedSubject,
			ReserveItems:       reserveItemsSubject,
			ConfirmOrder:       confirmOrderSubject,
			ReleaseReservation: releaseReservationSubject,
			OrderFinalized:     orderFinalizedSubject,
			OrderCancelled:     orderCancelledSubject,
			ItemsReserved:      itemsReservedSubject,
			ReservationFailed:  reservationFailedSubject,
			OrderConfirmed:     orderConfirmedSubject,
		},
	})
	require.NoError(t, err)

	e := &env{
		server:         orderStack.Server,
		orderStack:     orderStack,
		paymentsPool:   paymentsPool,
		analyticsPool:  analyticsPool,
		mongoDB:        mongoDB,
		minioClient:    minioClient,
		natsConn:       natsConn,
		storeWorker:    storeWorker,
		notification:   notification,
		analytics:      analytics,
		paymentGRPC:    paymentGRPC,
		pgContainer:    pgContainer,
		redisContainer: redisContainer,
		natsContainer:  natsContainer,
		mongoContainer: mongoContainer,
		minioContainer: minioContainer,
	}

	e.token = e.login(t)
	t.Cleanup(func() {
		orderStack.Close()
		storeWorker.Close()
		notification.Close()
		analytics.Close()
		paymentGRPC.Close()
		natsConn.Close()
		_ = redisClient.Close()
		_ = pgContainer.Terminate(context.Background())
		_ = redisContainer.Terminate(context.Background())
		_ = natsContainer.Terminate(context.Background())
		_ = mongoContainer.Terminate(context.Background())
		_ = minioContainer.Terminate(context.Background())
	})
	return e
}

func startPostgres(t *testing.T, ctx context.Context) (testcontainers.Container, string, string, string) {
	t.Helper()
	pgContainer, err := tcpostgres.Run(
		ctx,
		"postgres:18.4-alpine",
		tcpostgres.WithDatabase("orders"),
		tcpostgres.WithUsername("orders"),
		tcpostgres.WithPassword("orders"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err)

	baseConn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	adminPool, err := pgxpool.New(ctx, baseConn)
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, `CREATE DATABASE payments`)
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, `CREATE DATABASE analytics`)
	require.NoError(t, err)
	adminPool.Close()

	ordersConn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	paymentsConn := ordersConn[:len(ordersConn)-len("/orders?sslmode=disable")] + "/payments?sslmode=disable"
	analyticsConn := ordersConn[:len(ordersConn)-len("/orders?sslmode=disable")] + "/analytics?sslmode=disable"
	return pgContainer, ordersConn, paymentsConn, analyticsConn
}

func startRedis(t *testing.T, ctx context.Context) (*goredis.Client, testcontainers.Container) {
	t.Helper()
	redisContainer, err := tcredis.Run(ctx, "redis:8.8-alpine")
	require.NoError(t, err)
	redisConnStr, err := redisContainer.ConnectionString(ctx)
	require.NoError(t, err)
	redisOpts, err := goredis.ParseURL(redisConnStr)
	require.NoError(t, err)
	client := goredis.NewClient(redisOpts)
	require.NoError(t, client.Ping(ctx).Err())
	return client, redisContainer
}

func startNATS(t *testing.T, ctx context.Context) (*natspkg.Conn, testcontainers.Container) {
	t.Helper()
	natsContainer, err := tcnats.Run(ctx, "nats:2.14-alpine")
	require.NoError(t, err)
	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)
	nc, err := natspkg.Connect(natsURL)
	require.NoError(t, err)
	return nc, natsContainer
}

func startMongo(t *testing.T, ctx context.Context) (*mongo.Database, testcontainers.Container) {
	t.Helper()
	mongoContainer, err := tcmongo.Run(ctx, "mongo:8.0")
	require.NoError(t, err)
	mongoURI, err := mongoContainer.ConnectionString(ctx)
	require.NoError(t, err)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err)
	return client.Database("store"), mongoContainer
}

func startMinIO(t *testing.T, ctx context.Context) (string, *minio.Client, testcontainers.Container) {
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
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "9000/tcp")
	require.NoError(t, err)
	endpoint := host + ":" + port.Port()
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
		Secure: false,
	})
	require.NoError(t, err)
	require.NoError(t, client.MakeBucket(ctx, minioBucket, minio.MakeBucketOptions{}))
	return endpoint, client, container
}

func (e *env) login(t *testing.T) string {
	t.Helper()
	resp := e.doJSON(t, http.MethodPost, "/auth/login", "", fmt.Sprintf(
		`{"email":"%s","password":"%s"}`,
		demoEmail,
		demoPassword,
	))
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotEmpty(t, body.AccessToken)
	return body.AccessToken
}

func (e *env) doJSON(t *testing.T, method, path, token, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, e.server.URL+path, reader)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func (e *env) waitForReservedCart(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		return e.orderStack.CartAllItemsReserved(context.Background(), demoUserID)
	}, 10*time.Second, pollInterval)
}

func (e *env) waitForOrderStatus(t *testing.T, orderID, status string) {
	t.Helper()
	require.Eventually(t, func() bool {
		resp := e.doJSON(t, http.MethodGet, "/orders/"+orderID, e.token, "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		var order struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
			return false
		}
		return order.Status == status
	}, 15*time.Second, pollInterval)
}

func (e *env) checkoutOrder(t *testing.T) string {
	t.Helper()
	addResp := e.doJSON(t, http.MethodPost, "/cart/items", e.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":%d}`,
		testProductID,
		testUnitPrice,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()
	e.waitForReservedCart(t)

	checkoutResp := e.doJSON(t, http.MethodPost, "/orders", e.token, `{"delivery_address":"Moscow"}`)
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)
	var order struct {
		OrderID string `json:"order_id"`
		Status  string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))
	_ = checkoutResp.Body.Close()
	require.Equal(t, "RESERVED", order.Status)
	require.NotEmpty(t, order.OrderID)
	return order.OrderID
}

func (e *env) breakStoreConfirm(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := e.mongoDB.Collection("stock").UpdateOne(ctx,
		bson.M{"product_id": testProductID},
		bson.M{"$set": bson.M{"reserved": 0}},
	)
	require.NoError(t, err)
}

func (e *env) stockReserved(t *testing.T) int {
	t.Helper()
	var doc struct {
		Reserved int `bson:"reserved"`
	}
	err := e.mongoDB.Collection("stock").FindOne(context.Background(), bson.M{"product_id": testProductID}).Decode(&doc)
	require.NoError(t, err)
	return doc.Reserved
}

func (e *env) countProcessedOrders(t *testing.T, orderID string) int {
	t.Helper()
	var count int
	err := e.analyticsPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM processed_orders WHERE order_id = $1`, orderID).Scan(&count)
	require.NoError(t, err)
	return count
}

func (e *env) countPaymentTransactions(t *testing.T, orderID string) int {
	t.Helper()
	var count int
	err := e.paymentsPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM transactions WHERE order_id = $1`, orderID).Scan(&count)
	require.NoError(t, err)
	return count
}

func (e *env) publishDuplicateFinalized(t *testing.T, orderID string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"order_id":     orderID,
		"user_id":      demoUserID,
		"total_amount": testUnitPrice,
		"status":       "CONFIRMED",
		"finalized_at": time.Now().UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.NoError(t, e.natsConn.Publish(orderFinalizedSubject, payload))
	require.NoError(t, e.natsConn.Flush())
}
