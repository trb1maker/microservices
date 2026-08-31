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
	"uuid"

	"github.com/trb1maker/microservices/internal/order-service/app"
	"github.com/trb1maker/microservices/internal/order-service/domain"
	"github.com/trb1maker/microservices/internal/order-service/migrations"
	"github.com/trb1maker/microservices/internal/platform/health"
	"github.com/trb1maker/microservices/tests/internal/natstest"

	cartredis "github.com/trb1maker/microservices/internal/order-service/adapters/cart_repository/redis"
	checkoutpostgres "github.com/trb1maker/microservices/internal/order-service/adapters/checkout_writer/postgres"
	natsconsumer "github.com/trb1maker/microservices/internal/order-service/adapters/event_consumer/nats"
	natsadapter "github.com/trb1maker/microservices/internal/order-service/adapters/event_publisher/nats"
	httpadapter "github.com/trb1maker/microservices/internal/order-service/adapters/http"
	orderpostgres "github.com/trb1maker/microservices/internal/order-service/adapters/order_repository/postgres"
	paymentgrpc "github.com/trb1maker/microservices/internal/order-service/adapters/payment/grpc"
	userpostgres "github.com/trb1maker/microservices/internal/order-service/adapters/user_repository/postgres"
	"github.com/trb1maker/microservices/internal/platform/outbox"
	outboxnats "github.com/trb1maker/microservices/internal/platform/outbox/natspub"
	outboxpg "github.com/trb1maker/microservices/internal/platform/outbox/pgstore"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/trb1maker/microservices/internal/platform/natsx"
)

const (
	orderCreatedSubject       = "orders.created"
	orderCancelledSubject     = "orders.cancelled"
	releaseReservationSubject = "cart.release_reservation"
	reserveItemsSubject       = "cart.reserve_items"
	itemsReservedSubject      = "store.items_reserved"
	confirmOrderSubject       = "orders.confirm"
	orderConfirmedSubject     = "store.order_confirmed"
	orderFinalizedSubject     = "orders.finalized"
	startupTimeout            = 2 * time.Minute
)

var testJWTSecret = envOr("JWT_SECRET", "dev-jwt-secret-minimum-32-characters-long")

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type testEnv struct {
	server         *httptest.Server
	pool           *pgxpool.Pool
	redis          *goredis.Client
	natsClient     *natsx.Client
	cartRepo       app.CartRepository
	pgContainer    testcontainers.Container
	redisContainer testcontainers.Container
	natsContainer  testcontainers.Container
	token          string
	userID         string
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func newTestEnv(t *testing.T, paymentClient ...app.PaymentClient) *testEnv {
	t.Helper()

	var payment app.PaymentClient = app.NewNoopPaymentClient()
	if len(paymentClient) > 0 && paymentClient[0] != nil {
		payment = paymentClient[0]
	}

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	consumerCtx := context.WithoutCancel(ctx)

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

	natsContainer, err := tcnats.Run(ctx, natstest.Image, natstest.ContainerOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })

	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	natsClient := natstest.NewClient(t, natsURL)
	t.Cleanup(natsClient.Conn().Close)

	relay := outbox.NewRelay(outboxpg.New(pool), outboxnats.New(natsClient), outbox.RelayConfig{
		PollInterval: 100 * time.Millisecond,
		BatchSize:    50,
	})
	relayCtx, relayCancel := context.WithCancel(consumerCtx)
	go func() { _ = relay.Run(relayCtx) }()
	t.Cleanup(relayCancel)

	_, err = natsClient.ConsumeDurable(consumerCtx, "CART", "test-mock-store-reserve", reserveItemsSubject, func(_ context.Context, msg *nats.Msg) error {
		var req struct {
			UserID    string `json:"user_id"`
			ProductID string `json:"product_id"`
			Quantity  int    `json:"quantity"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil
		}
		payload, err := json.Marshal(app.ItemsReserved{
			OrderID:   msg.Header.Get("X-Order-ID"),
			UserID:    req.UserID,
			ProductID: req.ProductID,
			Quantity:  req.Quantity,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil
		}
		return natsClient.Publish(context.Background(), itemsReservedSubject, payload)
	}, natsx.DurableConsumerConfig{})
	require.NoError(t, err)

	orderRepo := orderpostgres.NewOrderRepository(pool)
	cartRepo := cartredis.NewCartRepository(redisClient, 24*time.Hour)
	userRepo := userpostgres.NewUserRepository(pool)
	authService := app.NewAuthService(userRepo, testJWTSecret, time.Hour)
	events := natsadapter.NewPublisher(natsClient, natsadapter.Subjects{
		OrderCreated:       orderCreatedSubject,
		ReserveItems:       reserveItemsSubject,
		ConfirmOrder:       confirmOrderSubject,
		ReleaseReservation: releaseReservationSubject,
		OrderFinalized:     orderFinalizedSubject,
		OrderCancelled:     orderCancelledSubject,
	})

	cartService := app.NewCartService(cartRepo, events)
	checkoutWriter := checkoutpostgres.NewWriter(pool)
	orderService := app.NewOrderService(
		cartRepo,
		orderRepo,
		events,
		payment,
		app.NewNoopOrderMetrics(),
		checkoutWriter,
		app.OrderCreatedSubject(orderCreatedSubject),
		app.OrderEventSubjects{
			ConfirmOrder:   confirmOrderSubject,
			OrderFinalized: orderFinalizedSubject,
			OrderCancelled: orderCancelledSubject,
		},
	)
	consumer := natsconsumer.NewConsumer(natsClient, natsconsumer.Subjects{
		ItemsReserved:     itemsReservedSubject,
		ReservationFailed: "store.reservation_failed",
		OrderConfirmed:    orderConfirmedSubject,
	}, cartService, orderService)
	require.NoError(t, consumer.Start(consumerCtx))
	t.Cleanup(consumer.Close)

	checks := map[string]health.CheckFunc{
		"postgres": orderRepo.Ping,
		"redis":    cartRepo.Ping,
		"nats": func(context.Context) error {
			if !natsClient.Conn().IsConnected() {
				return fmt.Errorf("nats is not connected")
			}
			return nil
		},
	}

	handler := httpadapter.NewHandler(cartService, orderService, health.NewChecker(checks), nil)
	server := httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{
		Addr: ":8080",
		Auth: &httpadapter.AuthConfig{JWTSecret: testJWTSecret},
	}, handler, httpadapter.NewAppAuthAdapter(authService), nil).Handler)
	t.Cleanup(server.Close)

	env := &testEnv{
		server:         server,
		pool:           pool,
		redis:          redisClient,
		natsClient:     natsClient,
		cartRepo:       cartRepo,
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

func (env *testEnv) subscribeJS(t *testing.T, subject string, durable string, handler natsx.Handler) {
	t.Helper()
	stream := natsx.StreamForSubject(subject)
	_, err := env.natsClient.ConsumeDurable(context.Background(), stream, durable, subject, handler, natsx.DurableConsumerConfig{})
	require.NoError(t, err)
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

	productID := uuid.NewV7().String()

	eventCh := make(chan []byte, 1)
	env.subscribeJS(t, orderCreatedSubject, "test-checkout-order-created", func(_ context.Context, msg *nats.Msg) error {
		payload := make([]byte, len(msg.Data))
		copy(payload, msg.Data)
		select {
		case eventCh <- payload:
		default:
		}
		return nil
	})

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()
	env.waitForReservedCart(t)

	checkoutResp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Moscow"}`)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)

	var order struct {
		OrderID    string `json:"order_id"`
		Status     string `json:"status"`
		TotalPrice int64  `json:"total_price"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))
	assert.Equal(t, "RESERVED", order.Status)
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
	env.subscribeJS(t, orderCreatedSubject, "test-empty-cart-order-created", func(_ context.Context, _ *nats.Msg) error {
		select {
		case eventCh <- struct{}{}:
		default:
		}
		return nil
	})

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

	productID := uuid.NewV7().String()

	cancelledCh := make(chan []byte, 1)
	releaseCh := make(chan []byte, 1)

	env.subscribeJS(t, orderCancelledSubject, "test-cancel-order-cancelled", func(_ context.Context, msg *nats.Msg) error {
		select {
		case cancelledCh <- msg.Data:
		default:
		}
		return nil
	})

	env.subscribeJS(t, releaseReservationSubject, "test-cancel-order-release", func(_ context.Context, msg *nats.Msg) error {
		select {
		case releaseCh <- msg.Data:
		default:
		}
		return nil
	})

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()
	env.waitForReservedCart(t)

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
	productID := uuid.NewV7().String()

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()
	env.waitForReservedCart(t)

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

	productID := uuid.NewV7().String()

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

func TestIntegration_CartTTL(t *testing.T) {
	env := newTestEnv(t)

	productID := uuid.NewV7().String()
	resp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	pttl, err := env.redis.PTTL(context.Background(), "cart:"+env.userID).Result()
	require.NoError(t, err)
	assert.Greater(t, pttl, time.Duration(0))
}

// Store не поднимается: orderConfirmed публикуется в NATS вручную.
func TestIntegration_PayToConfirmed_SimulatedStoreConfirm(t *testing.T) {
	env := newTestEnv(t)
	productID := uuid.NewV7().String()

	finalizedCh := make(chan []byte, 1)
	env.subscribeJS(t, orderFinalizedSubject, "test-pay-confirmed-finalized", func(_ context.Context, msg *nats.Msg) error {
		select {
		case finalizedCh <- msg.Data:
		default:
		}
		return nil
	})

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()
	env.waitForReservedCart(t)

	checkoutResp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Moscow"}`)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)

	var order struct {
		OrderID string `json:"order_id"`
		Status  string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))
	assert.Equal(t, "RESERVED", order.Status)

	payResp := env.doJSON(t, http.MethodPost, "/orders/"+order.OrderID+"/pay", env.token, "")
	t.Cleanup(func() { _ = payResp.Body.Close() })
	require.Equal(t, http.StatusOK, payResp.StatusCode)

	var paid struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(payResp.Body).Decode(&paid))
	assert.Equal(t, "PAID", paid.Status)

	confirmedPayload, err := json.Marshal(app.OrderConfirmed{
		OrderID:   order.OrderID,
		UserID:    env.userID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.NoError(t, env.natsClient.Publish(context.Background(), orderConfirmedSubject, confirmedPayload))

	require.Eventually(t, func() bool {
		getResp := env.doJSON(t, http.MethodGet, "/orders/"+order.OrderID, env.token, "")
		defer getResp.Body.Close()
		if getResp.StatusCode != http.StatusOK {
			return false
		}
		var current struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(getResp.Body).Decode(&current); err != nil {
			return false
		}
		return current.Status == "CONFIRMED"
	}, 5*time.Second, 100*time.Millisecond)

	select {
	case payload := <-finalizedCh:
		var event struct {
			OrderID     string `json:"order_id"`
			TotalAmount int64  `json:"total_amount"`
			Status      string `json:"status"`
			FinalizedAt string `json:"finalized_at"`
		}
		require.NoError(t, json.Unmarshal(payload, &event))
		assert.Equal(t, order.OrderID, event.OrderID)
		assert.Equal(t, int64(100), event.TotalAmount)
		assert.Equal(t, "CONFIRMED", event.Status)
		assert.NotEmpty(t, event.FinalizedAt)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for ORDER_FINALIZED event")
	}
}

func TestIntegration_CancelPaidOrder(t *testing.T) {
	env := newTestEnv(t)
	productID := uuid.NewV7().String()

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()
	env.waitForReservedCart(t)

	checkoutResp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Moscow"}`)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)

	var order struct {
		OrderID string `json:"order_id"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))

	payResp := env.doJSON(t, http.MethodPost, "/orders/"+order.OrderID+"/pay", env.token, "")
	t.Cleanup(func() { _ = payResp.Body.Close() })
	require.Equal(t, http.StatusOK, payResp.StatusCode)

	cancelResp := env.doJSON(t, http.MethodDelete, "/orders/"+order.OrderID, env.token, "")
	t.Cleanup(func() { _ = cancelResp.Body.Close() })
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	var cancelled struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(cancelResp.Body).Decode(&cancelled))
	assert.Equal(t, "CANCELLED", cancelled.Status)
}

func TestIntegration_ListOrders(t *testing.T) {
	env := newTestEnv(t)
	productID := uuid.NewV7().String()

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()
	env.waitForReservedCart(t)

	checkoutResp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Moscow"}`)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)
	_ = checkoutResp.Body.Close()

	listResp := env.doJSON(t, http.MethodGet, "/orders", env.token, "")
	t.Cleanup(func() { _ = listResp.Body.Close() })
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var body struct {
		Orders []struct {
			Status string `json:"status"`
		} `json:"orders"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&body))
	require.Len(t, body.Orders, 1)
	assert.Equal(t, "RESERVED", body.Orders[0].Status)
}

func TestIntegration_OutboxCheckoutRelay(t *testing.T) {
	env := newOutboxTestEnv(t)

	productID := uuid.NewV7().String()
	eventCh := make(chan []byte, 1)
	env.subscribeJS(t, orderCreatedSubject, "test-outbox-order-created", func(_ context.Context, msg *nats.Msg) error {
		payload := make([]byte, len(msg.Data))
		copy(payload, msg.Data)
		select {
		case eventCh <- payload:
		default:
		}
		return nil
	})

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()
	env.waitForReservedCart(t)

	checkoutResp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Outbox Street"}`)
	t.Cleanup(func() { _ = checkoutResp.Body.Close() })
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)

	var order struct {
		OrderID string `json:"order_id"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))
	require.NotEmpty(t, order.OrderID)

	require.Eventually(t, func() bool {
		var unpublished int64
		if err := env.pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM outbox WHERE published_at IS NULL`).Scan(&unpublished); err != nil {
			return false
		}
		return unpublished == 0
	}, 5*time.Second, 100*time.Millisecond)

	var receivedPayload []byte
	require.Eventually(t, func() bool {
		select {
		case payload := <-eventCh:
			receivedPayload = append([]byte(nil), payload...)
			return true
		default:
			return false
		}
	}, 5*time.Second, 100*time.Millisecond)

	var created app.OrderCreated
	require.NoError(t, json.Unmarshal(receivedPayload, &created))
	assert.Equal(t, order.OrderID, created.OrderID)
}

func newOutboxTestEnv(t *testing.T) *testEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	consumerCtx := context.WithoutCancel(ctx)

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

	natsContainer, err := tcnats.Run(ctx, natstest.Image, natstest.ContainerOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })

	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)
	natsClient := natstest.NewClient(t, natsURL)
	t.Cleanup(natsClient.Conn().Close)

	_, err = natsClient.ConsumeDurable(consumerCtx, "CART", "test-outbox-store-reserve", reserveItemsSubject, func(_ context.Context, msg *nats.Msg) error {
		var req struct {
			UserID    string `json:"user_id"`
			ProductID string `json:"product_id"`
			Quantity  int    `json:"quantity"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil
		}
		payload, err := json.Marshal(app.ItemsReserved{
			OrderID:   msg.Header.Get("X-Order-ID"),
			UserID:    req.UserID,
			ProductID: req.ProductID,
			Quantity:  req.Quantity,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil
		}
		return natsClient.Publish(context.Background(), itemsReservedSubject, payload)
	}, natsx.DurableConsumerConfig{})
	require.NoError(t, err)

	orderRepo := orderpostgres.NewOrderRepository(pool)
	cartRepo := cartredis.NewCartRepository(redisClient, 24*time.Hour)
	userRepo := userpostgres.NewUserRepository(pool)
	authService := app.NewAuthService(userRepo, testJWTSecret, time.Hour)
	events := natsadapter.NewPublisher(natsClient, natsadapter.Subjects{
		OrderCreated:       orderCreatedSubject,
		ReserveItems:       reserveItemsSubject,
		ConfirmOrder:       confirmOrderSubject,
		ReleaseReservation: releaseReservationSubject,
		OrderFinalized:     orderFinalizedSubject,
		OrderCancelled:     orderCancelledSubject,
	})

	checkoutWriter := checkoutpostgres.NewWriter(pool)
	relay := outbox.NewRelay(outboxpg.New(pool), outboxnats.New(natsClient), outbox.RelayConfig{
		PollInterval: 100 * time.Millisecond,
		BatchSize:    50,
	})
	relayCtx, relayCancel := context.WithCancel(context.Background())
	t.Cleanup(relayCancel)
	go func() { _ = relay.Run(relayCtx) }()

	cartService := app.NewCartService(cartRepo, events)
	orderService := app.NewOrderService(
		cartRepo,
		orderRepo,
		events,
		app.NewNoopPaymentClient(),
		app.NewNoopOrderMetrics(),
		checkoutWriter,
		app.OrderCreatedSubject(orderCreatedSubject),
		app.OrderEventSubjects{
			ConfirmOrder:   confirmOrderSubject,
			OrderFinalized: orderFinalizedSubject,
			OrderCancelled: orderCancelledSubject,
		},
	)
	consumer := natsconsumer.NewConsumer(natsClient, natsconsumer.Subjects{
		ItemsReserved:     itemsReservedSubject,
		ReservationFailed: "store.reservation_failed",
		OrderConfirmed:    orderConfirmedSubject,
	}, cartService, orderService)
	require.NoError(t, consumer.Start(consumerCtx))
	t.Cleanup(consumer.Close)

	checks := map[string]health.CheckFunc{
		"postgres": orderRepo.Ping,
		"redis":    cartRepo.Ping,
		"nats": func(context.Context) error {
			if !natsClient.Conn().IsConnected() {
				return fmt.Errorf("nats is not connected")
			}
			return nil
		},
	}

	handler := httpadapter.NewHandler(cartService, orderService, health.NewChecker(checks), nil)
	server := httptest.NewServer(httpadapter.NewServer(httpadapter.ServerConfig{
		Addr: ":8080",
		Auth: &httpadapter.AuthConfig{JWTSecret: testJWTSecret},
	}, handler, httpadapter.NewAppAuthAdapter(authService), nil).Handler)
	t.Cleanup(server.Close)

	env := &testEnv{
		server:         server,
		pool:           pool,
		redis:          redisClient,
		natsClient:     natsClient,
		cartRepo:       cartRepo,
		pgContainer:    pgContainer,
		redisContainer: redisContainer,
		natsContainer:  natsContainer,
		userID:         "11111111-1111-4111-8111-111111111111",
	}
	env.token = env.login(t, "demo@example.com", "demo123")
	return env
}

func (env *testEnv) waitForReservedCart(t *testing.T) {
	t.Helper()

	userID := domain.UserID(uuid.MustParse(env.userID))
	require.Eventually(t, func() bool {
		cart, err := env.cartRepo.Get(context.Background(), userID)
		if err != nil || cart == nil {
			return false
		}
		return cart.AllItemsReserved()
	}, 5*time.Second, 50*time.Millisecond)
}

func TestIntegration_PayOrder_PaymentUnavailable(t *testing.T) {
	deadPayment, err := paymentgrpc.NewPaymentClient(context.Background(), paymentgrpc.ClientConfig{
		Addr:       "127.0.0.1:1",
		Insecure:   true,
		RPCTimeout: 500 * time.Millisecond,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = deadPayment.Close() })

	env := newTestEnv(t, deadPayment)
	productID := uuid.NewV7().String()

	addResp := env.doJSON(t, http.MethodPost, "/cart/items", env.token, fmt.Sprintf(
		`{"product_id":"%s","quantity":1,"unit_price":100}`,
		productID,
	))
	require.Equal(t, http.StatusCreated, addResp.StatusCode)
	_ = addResp.Body.Close()
	env.waitForReservedCart(t)

	checkoutResp := env.doJSON(t, http.MethodPost, "/orders", env.token, `{"delivery_address":"Moscow"}`)
	require.Equal(t, http.StatusCreated, checkoutResp.StatusCode)
	var order struct {
		OrderID string `json:"order_id"`
		Status  string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(checkoutResp.Body).Decode(&order))
	_ = checkoutResp.Body.Close()
	assert.Equal(t, "RESERVED", order.Status)

	start := time.Now()
	payResp := env.doJSON(t, http.MethodPost, "/orders/"+order.OrderID+"/pay", env.token, "")
	elapsed := time.Since(start)
	t.Cleanup(func() { _ = payResp.Body.Close() })
	assert.Equal(t, http.StatusServiceUnavailable, payResp.StatusCode)
	assert.Less(t, elapsed, 2*time.Second)

	var current struct {
		Status string `json:"status"`
	}
	getResp := env.doJSON(t, http.MethodGet, "/orders/"+order.OrderID, env.token, "")
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&current))
	_ = getResp.Body.Close()
	assert.Equal(t, "RESERVED", current.Status)
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
