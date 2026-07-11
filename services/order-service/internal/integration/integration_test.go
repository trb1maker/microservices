//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/migrations"

	"github.com/trb1maker/microservices/pkg/health"

	cartredis "github.com/trb1maker/microservices/services/order-service/internal/adapters/cart_repository/redis"
	natsadapter "github.com/trb1maker/microservices/services/order-service/internal/adapters/event_publisher/nats"
	httpadapter "github.com/trb1maker/microservices/services/order-service/internal/adapters/http"
	orderpostgres "github.com/trb1maker/microservices/services/order-service/internal/adapters/order_repository/postgres"
	userpostgres "github.com/trb1maker/microservices/services/order-service/internal/adapters/user_repository/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	natspkg "github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	orderCreatedSubject       = "orders.created"
	orderCancelledSubject     = "orders.cancelled"
	releaseReservationSubject = "cart.release_reservation"
	startupTimeout            = 2 * time.Minute
	testJWTSecret             = "integration-test-secret-minimum-32-characters"
)

type testEnv struct {
	server         *httptest.Server
	pool           *pgxpool.Pool
	redis          *goredis.Client
	natsConn       *natspkg.Conn
	pgContainer    testcontainers.Container
	redisContainer testcontainers.Container
	natsContainer  testcontainers.Container
	token          string
	userID         string
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()

	pgContainer, err := postgres.Run(
		ctx,
		"postgres:18.4-alpine",
		postgres.WithDatabase("orders"),
		postgres.WithUsername("orders"),
		postgres.WithPassword("orders"),
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
	t.Cleanup(func() { _ = db.Close() })

	redisContainer, err := redis.Run(ctx, "redis:8.8-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = redisContainer.Terminate(context.Background()) })

	redisConnStr, err := redisContainer.ConnectionString(ctx)
	require.NoError(t, err)

	redisOpts, err := goredis.ParseURL(redisConnStr)
	require.NoError(t, err)

	redisClient := goredis.NewClient(redisOpts)
	t.Cleanup(func() { _ = redisClient.Close() })
	require.NoError(t, redisClient.Ping(ctx).Err())

	natsContainer, err := tcnats.Run(ctx, "nats:2.14-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })

	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	natsConn, err := natspkg.Connect(natsURL)
	require.NoError(t, err)
	t.Cleanup(natsConn.Close)

	orderRepo := orderpostgres.NewOrderRepository(pool)
	cartRepo := cartredis.NewCartRepository(redisClient)
	userRepo := userpostgres.NewUserRepository(pool)
	authService := app.NewAuthService(userRepo, testJWTSecret, time.Hour)
	events := natsadapter.NewPublisher(natsConn, natsadapter.Subjects{
		OrderCreated:       orderCreatedSubject,
		ReserveItems:       "cart.reserve_items",
		ConfirmOrder:       "orders.confirm",
		ReleaseReservation: "cart.release_reservation",
		OrderFinalized:     "orders.finalized",
		OrderCancelled:     "orders.cancelled",
	})

	checks := map[string]health.CheckFunc{
		"postgres": orderRepo.Ping,
		"redis":    cartRepo.Ping,
		"nats": func(context.Context) error {
			if !natsConn.IsConnected() {
				return fmt.Errorf("nats is not connected")
			}
			return nil
		},
	}

	cartService := app.NewCartService(cartRepo)
	orderService := app.NewOrderService(cartRepo, orderRepo, events, app.NewNoopOrderMetrics())
	handler := httpadapter.NewHandler(cartService, orderService, health.NewChecker(checks))
	server := httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{
		Addr: ":8080",
		Auth: &httpadapter.AuthConfig{JWTSecret: testJWTSecret},
	}, handler, httpadapter.NewAppAuthAdapter(authService), nil).Handler)
	t.Cleanup(server.Close)

	env := &testEnv{
		server:         server,
		pool:           pool,
		redis:          redisClient,
		natsConn:       natsConn,
		pgContainer:    pgContainer,
		redisContainer: redisContainer,
		natsContainer:  natsContainer,
		userID:         "11111111-1111-4111-8111-111111111111",
	}

	env.token = env.login(t, "demo@example.com", "demo123")
	return env
}

func (env *testEnv) login(t *testing.T, email, password string) string {
	t.Helper()

	resp := env.doJSON(t, http.MethodPost, "/auth/login", "", fmt.Sprintf(
		`{"email":"%s","password":"%s"}`,
		email,
		password,
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

func TestIntegration_Ready(t *testing.T) {
	env := newTestEnv(t)

	resp, err := http.Get(env.server.URL + "/ready")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ready", body.Status)
	assert.Equal(t, "ok", body.Checks["postgres"])
	assert.Equal(t, "ok", body.Checks["redis"])
	assert.Equal(t, "ok", body.Checks["nats"])
}

func TestIntegration_CheckoutHappyPath(t *testing.T) {
	env := newTestEnv(t)

	productID := uuid.New().String()

	eventCh := make(chan []byte, 1)
	sub, err := env.natsConn.Subscribe(orderCreatedSubject, func(msg *natspkg.Msg) {
		payload := make([]byte, len(msg.Data))
		copy(payload, msg.Data)
		select {
		case eventCh <- payload:
		default:
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()

	checkoutResp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Moscow"}`)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)

	var order struct {
		OrderID    string `json:"order_id"`
		Status     string `json:"status"`
		TotalPrice int64  `json:"total_price"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))
	assert.Equal(t, "PENDING", order.Status)
	assert.Equal(t, int64(100), order.TotalPrice)

	cartKey := "cart:" + env.userID
	exists, err := env.redis.Exists(context.Background(), cartKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)

	var itemsCount int
	err = env.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM order_items WHERE order_id = $1",
		order.OrderID,
	).Scan(&itemsCount)
	require.NoError(t, err)
	assert.Equal(t, 1, itemsCount)

	select {
	case payload := <-eventCh:
		var event struct {
			OrderID    string `json:"order_id"`
			UserID     string `json:"user_id"`
			TotalPrice int64  `json:"total_price"`
		}
		require.NoError(t, json.Unmarshal(payload, &event))
		assert.Equal(t, order.OrderID, event.OrderID)
		assert.Equal(t, env.userID, event.UserID)
		assert.Equal(t, int64(100), event.TotalPrice)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for ORDER_CREATED event")
	}

	getCartResp := env.doJSON(t, http.MethodGet, "/cart", env.token, "")
	t.Cleanup(func() { _ = getCartResp.Body.Close() })
	require.Equal(t, http.StatusOK, getCartResp.StatusCode)

	var cart struct {
		Items []any `json:"items"`
	}
	require.NoError(t, json.NewDecoder(getCartResp.Body).Decode(&cart))
	assert.Empty(t, cart.Items)

	getOrderResp := env.doJSON(t, http.MethodGet, "/orders/"+order.OrderID, env.token, "")
	t.Cleanup(func() { _ = getOrderResp.Body.Close() })
	require.Equal(t, http.StatusOK, getOrderResp.StatusCode)
}

func TestIntegration_CheckoutEmptyCart(t *testing.T) {
	env := newTestEnv(t)

	eventCh := make(chan struct{}, 1)
	sub, err := env.natsConn.Subscribe(orderCreatedSubject, func(_ *natspkg.Msg) {
		select {
		case eventCh <- struct{}{}:
		default:
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	resp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Moscow"}`)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	select {
	case <-eventCh:
		t.Fatal("unexpected ORDER_CREATED event")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestIntegration_CancelOrder(t *testing.T) {
	env := newTestEnv(t)

	productID := uuid.New().String()

	cancelledCh := make(chan []byte, 1)
	releaseCh := make(chan []byte, 1)

	cancelSub, err := env.natsConn.Subscribe(orderCancelledSubject, func(msg *natspkg.Msg) {
		select {
		case cancelledCh <- msg.Data:
		default:
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cancelSub.Unsubscribe() })

	releaseSub, err := env.natsConn.Subscribe(releaseReservationSubject, func(msg *natspkg.Msg) {
		select {
		case releaseCh <- msg.Data:
		default:
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = releaseSub.Unsubscribe() })

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()

	checkoutResp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Moscow"}`)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)

	var order struct {
		OrderID string `json:"order_id"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))

	cancelResp := env.doJSON(t, http.MethodDelete, "/orders/"+order.OrderID, env.token, "")
	t.Cleanup(func() { _ = cancelResp.Body.Close() })
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	var cancelled struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(cancelResp.Body).Decode(&cancelled))
	assert.Equal(t, "CANCELLED", cancelled.Status)

	select {
	case payload := <-cancelledCh:
		var event struct {
			OrderID string `json:"order_id"`
			UserID  string `json:"user_id"`
		}
		require.NoError(t, json.Unmarshal(payload, &event))
		assert.Equal(t, order.OrderID, event.OrderID)
		assert.Equal(t, env.userID, event.UserID)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for ORDER_CANCELLED event")
	}

	select {
	case payload := <-releaseCh:
		var event struct {
			OrderID string `json:"order_id"`
			UserID  string `json:"user_id"`
		}
		require.NoError(t, json.Unmarshal(payload, &event))
		assert.Equal(t, order.OrderID, event.OrderID)
		assert.Equal(t, env.userID, event.UserID)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RELEASE_RESERVATION event")
	}
}

func TestIntegration_GetOrder_wrongUser(t *testing.T) {
	env := newTestEnv(t)

	otherToken := env.login(t, "admin@example.com", "admin123")
	productID := uuid.New().String()

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()

	checkoutResp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Moscow"}`)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)

	var order struct {
		OrderID string `json:"order_id"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))

	getResp := env.doJSON(t, http.MethodGet, "/orders/"+order.OrderID, otherToken, "")
	t.Cleanup(func() { _ = getResp.Body.Close() })
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

func TestIntegration_CartUpdatedAtRoundTrip(t *testing.T) {
	env := newTestEnv(t)

	productID := uuid.New().String()

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)

	var addBody struct {
		UpdatedAt string `json:"updated_at"`
	}
	require.NoError(t, json.NewDecoder(addResp.Body).Decode(&addBody))
	_ = addResp.Body.Close()

	getResp := env.doJSON(t, http.MethodGet, "/cart", env.token, "")
	t.Cleanup(func() { _ = getResp.Body.Close() })
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var cart struct {
		UpdatedAt string `json:"updated_at"`
	}
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&cart))
	assert.Equal(t, addBody.UpdatedAt, cart.UpdatedAt)
}

func (env *testEnv) doJSON(t *testing.T, method, path, token, body string) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, env.server.URL+path, reader)
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
