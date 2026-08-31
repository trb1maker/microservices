//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mongocontainer "github.com/testcontainers/testcontainers-go/modules/mongodb"
	natscontainer "github.com/testcontainers/testcontainers-go/modules/nats"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/trb1maker/microservices/internal/platform/natsx"
	mongoadapter "github.com/trb1maker/microservices/internal/store-service/adapters/mongodb"
	natsadapter "github.com/trb1maker/microservices/internal/store-service/adapters/nats"
	"github.com/trb1maker/microservices/internal/store-service/app"
	"github.com/trb1maker/microservices/tests/internal/natstest"
)

const (
	reserveItemsSubj        = "cart.reserve_items"
	confirmOrderSubj        = "orders.confirm"
	releaseReservationSubj  = "cart.release_reservation"
	itemsReservedSubj       = "store.items_reserved"
	reservationFailedSubj   = "store.reservation_failed"
	orderConfirmedSubj      = "store.order_confirmed"
	reservationReleasedSubj = "store.reservation_released"
)

func setupTestInfra(t *testing.T) (*mongo.Database, *natsx.Client, func()) {
	t.Helper()

	ctx := context.Background()

	mongoC, err := mongocontainer.Run(ctx, "mongo:8.0")
	require.NoError(t, err)

	mongoURI, err := mongoC.ConnectionString(ctx)
	require.NoError(t, err)

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err)

	db := client.Database("test_store")
	require.NoError(t, mongoadapter.SeedProducts(ctx, db))

	natsC, err := natscontainer.Run(ctx, natstest.Image, natstest.ContainerOptions()...)
	require.NoError(t, err)

	natsURI, err := natsC.ConnectionString(ctx)
	require.NoError(t, err)

	nc := natstest.NewClient(t, natsURI, nats.Name("test-store"))

	cleanup := func() {
		nc.Conn().Close()
		if err := client.Disconnect(ctx); err != nil {
			t.Logf("disconnect mongo: %v", err)
		}
		if err := mongoC.Terminate(ctx); err != nil {
			t.Logf("terminate mongo: %v", err)
		}
		if err := natsC.Terminate(ctx); err != nil {
			t.Logf("terminate nats: %v", err)
		}
	}

	return db, nc, cleanup
}

func jsSubscribe(t *testing.T, client *natsx.Client, subject, durable string) chan *nats.Msg {
	t.Helper()
	ch := make(chan *nats.Msg, 1)
	_, err := client.ConsumeDurable(context.Background(), natsx.StreamForSubject(subject), durable, subject, func(_ context.Context, msg *nats.Msg) error {
		copyMsg := &nats.Msg{
			Subject: msg.Subject,
			Data:    append([]byte(nil), msg.Data...),
			Header:  msg.Header,
		}
		select {
		case ch <- copyMsg:
		default:
		}
		return nil
	}, natsx.DurableConsumerConfig{})
	require.NoError(t, err)
	return ch
}

func nextMessageFrom(ch chan *nats.Msg, timeout time.Duration) *nats.Msg {
	select {
	case msg := <-ch:
		return msg
	case <-time.After(timeout):
		return nil
	}
}

func publishMsg(t *testing.T, client *natsx.Client, msg *nats.Msg) {
	t.Helper()
	require.NoError(t, client.PublishMsg(context.Background(), msg))
}

func publishPayload(t *testing.T, client *natsx.Client, subject string, payload []byte) {
	t.Helper()
	require.NoError(t, client.Publish(context.Background(), subject, payload))
}

func TestStoreService_ReserveItems_Success(t *testing.T) {
	db, client, cleanup := setupTestInfra(t)
	defer cleanup()

	productRepo := mongoadapter.NewProductRepository(db)
	stockRepo := mongoadapter.NewStockRepository(db)
	eventPub := natsadapter.NewEventPublisher(client, itemsReservedSubj, reservationFailedSubj, orderConfirmedSubj, reservationReleasedSubj)
	storeSvc := app.NewStoreService(productRepo, stockRepo, eventPub, nil)

	worker := natsadapter.NewWorker(storeSvc)
	err := worker.SubscribeAll(context.Background(), client, reserveItemsSubj, confirmOrderSubj, releaseReservationSubj)
	require.NoError(t, err)

	eventCh := jsSubscribe(t, client, itemsReservedSubj, "test-items-reserved")

	payload, _ := json.Marshal(map[string]any{
		"user_id":    "user-1",
		"product_id": "prod-1",
		"quantity":   2,
	})
	msg := nats.NewMsg(reserveItemsSubj)
	msg.Header.Set("X-Order-ID", "order-1")
	msg.Data = payload

	publishMsg(t, client, msg)

	eventMsg := nextMessageFrom(eventCh, 5*time.Second)
	require.NotNil(t, eventMsg, "expected items_reserved event")

	var event app.ItemsReservedEvent
	err = json.Unmarshal(eventMsg.Data, &event)
	require.NoError(t, err)

	assert.Equal(t, "order-1", event.OrderID)
	assert.Equal(t, "user-1", event.UserID)
	assert.Equal(t, "prod-1", event.ProductID)
	assert.Equal(t, 2, event.Quantity)

	// Verify stock was updated
	stock, err := stockRepo.Get(context.Background(), "prod-1")
	require.NoError(t, err)
	assert.Equal(t, 8, stock.Available)
	assert.Equal(t, 2, stock.Reserved)
}

func TestStoreService_ReserveItems_InsufficientStock(t *testing.T) {
	db, client, cleanup := setupTestInfra(t)
	defer cleanup()

	productRepo := mongoadapter.NewProductRepository(db)
	stockRepo := mongoadapter.NewStockRepository(db)
	eventPub := natsadapter.NewEventPublisher(client, itemsReservedSubj, reservationFailedSubj, orderConfirmedSubj, reservationReleasedSubj)
	storeSvc := app.NewStoreService(productRepo, stockRepo, eventPub, nil)

	worker := natsadapter.NewWorker(storeSvc)
	err := worker.SubscribeAll(context.Background(), client, reserveItemsSubj, confirmOrderSubj, releaseReservationSubj)
	require.NoError(t, err)

	eventCh := jsSubscribe(t, client, reservationFailedSubj, "test-reservation-failed")

	payload, _ := json.Marshal(map[string]any{
		"user_id":    "user-1",
		"product_id": "prod-1",
		"quantity":   100,
	})
	msg := nats.NewMsg(reserveItemsSubj)
	msg.Header.Set("X-Order-ID", "order-1")
	msg.Data = payload

	publishMsg(t, client, msg)

	eventMsg := nextMessageFrom(eventCh, 5*time.Second)
	require.NotNil(t, eventMsg, "expected reservation_failed event")

	var event app.ReservationFailedEvent
	err = json.Unmarshal(eventMsg.Data, &event)
	require.NoError(t, err)

	assert.Equal(t, "order-1", event.OrderID)
	assert.Equal(t, "user-1", event.UserID)
	assert.Equal(t, "prod-1", event.ProductID)
	assert.Equal(t, 100, event.Quantity)
	assert.Equal(t, "insufficient stock", event.Reason)

	// Verify stock was NOT changed
	stock, err := stockRepo.Get(context.Background(), "prod-1")
	require.NoError(t, err)
	assert.Equal(t, 10, stock.Available)
	assert.Equal(t, 0, stock.Reserved)
}

func TestStoreService_ConfirmOrder_Success(t *testing.T) {
	db, client, cleanup := setupTestInfra(t)
	defer cleanup()

	productRepo := mongoadapter.NewProductRepository(db)
	stockRepo := mongoadapter.NewStockRepository(db)
	eventPub := natsadapter.NewEventPublisher(client, itemsReservedSubj, reservationFailedSubj, orderConfirmedSubj, reservationReleasedSubj)
	storeSvc := app.NewStoreService(productRepo, stockRepo, eventPub, nil)

	worker := natsadapter.NewWorker(storeSvc)
	err := worker.SubscribeAll(context.Background(), client, reserveItemsSubj, confirmOrderSubj, releaseReservationSubj)
	require.NoError(t, err)

	// First reserve items
	payload, _ := json.Marshal(map[string]any{
		"user_id":    "user-1",
		"product_id": "prod-1",
		"quantity":   3,
	})
	msg := nats.NewMsg(reserveItemsSubj)
	msg.Header.Set("X-Order-ID", "order-1")
	msg.Data = payload

	publishMsg(t, client, msg)

	time.Sleep(500 * time.Millisecond)

	// Now confirm the order
	confirmPayload, _ := json.Marshal(map[string]any{
		"order_id":   "order-1",
		"user_id":    "user-1",
		"product_id": "prod-1",
		"quantity":   3,
	})

	publishPayload(t, client, confirmOrderSubj, confirmPayload)

	time.Sleep(500 * time.Millisecond)

	// Verify stock: reserved should be 0, available should be 7 (10-3)
	stock, err := stockRepo.Get(context.Background(), "prod-1")
	require.NoError(t, err)
	assert.Equal(t, 7, stock.Available)
	assert.Equal(t, 0, stock.Reserved)
}

func TestStoreService_ReleaseReservation_Success(t *testing.T) {
	db, client, cleanup := setupTestInfra(t)
	defer cleanup()

	productRepo := mongoadapter.NewProductRepository(db)
	stockRepo := mongoadapter.NewStockRepository(db)
	eventPub := natsadapter.NewEventPublisher(client, itemsReservedSubj, reservationFailedSubj, orderConfirmedSubj, reservationReleasedSubj)
	storeSvc := app.NewStoreService(productRepo, stockRepo, eventPub, nil)

	worker := natsadapter.NewWorker(storeSvc)
	err := worker.SubscribeAll(context.Background(), client, reserveItemsSubj, confirmOrderSubj, releaseReservationSubj)
	require.NoError(t, err)

	eventCh := jsSubscribe(t, client, reservationReleasedSubj, "test-reservation-released")

	payload, _ := json.Marshal(map[string]any{
		"user_id":    "user-1",
		"product_id": "prod-1",
		"quantity":   2,
	})
	msg := nats.NewMsg(reserveItemsSubj)
	msg.Header.Set("X-Order-ID", "order-1")
	msg.Data = payload

	publishMsg(t, client, msg)

	time.Sleep(500 * time.Millisecond)

	releasePayload, _ := json.Marshal(map[string]any{
		"order_id":   "order-1",
		"user_id":    "user-1",
		"product_id": "prod-1",
		"quantity":   2,
	})

	publishPayload(t, client, releaseReservationSubj, releasePayload)

	eventMsg := nextMessageFrom(eventCh, 5*time.Second)
	require.NotNil(t, eventMsg, "expected reservation_released event")

	var event app.ReservationReleasedEvent
	err = json.Unmarshal(eventMsg.Data, &event)
	require.NoError(t, err)

	assert.Equal(t, "order-1", event.OrderID)
	assert.Equal(t, "user-1", event.UserID)
	assert.Equal(t, "prod-1", event.ProductID)
	assert.Equal(t, 2, event.Quantity)

	// Verify stock: back to original
	stock, err := stockRepo.Get(context.Background(), "prod-1")
	require.NoError(t, err)
	assert.Equal(t, 10, stock.Available)
	assert.Equal(t, 0, stock.Reserved)
}

func TestStoreService_ProductNotFound(t *testing.T) {
	db, client, cleanup := setupTestInfra(t)
	defer cleanup()

	productRepo := mongoadapter.NewProductRepository(db)
	stockRepo := mongoadapter.NewStockRepository(db)
	eventPub := natsadapter.NewEventPublisher(client, itemsReservedSubj, reservationFailedSubj, orderConfirmedSubj, reservationReleasedSubj)
	storeSvc := app.NewStoreService(productRepo, stockRepo, eventPub, nil)

	worker := natsadapter.NewWorker(storeSvc)
	err := worker.SubscribeAll(context.Background(), client, reserveItemsSubj, confirmOrderSubj, releaseReservationSubj)
	require.NoError(t, err)

	eventCh := jsSubscribe(t, client, reservationFailedSubj, "test-product-not-found-failed")

	payload, _ := json.Marshal(map[string]any{
		"user_id":    "user-1",
		"product_id": "prod-999",
		"quantity":   1,
	})
	msg := nats.NewMsg(reserveItemsSubj)
	msg.Header.Set("X-Order-ID", "order-1")
	msg.Data = payload

	publishMsg(t, client, msg)

	eventMsg := nextMessageFrom(eventCh, 5*time.Second)
	require.NotNil(t, eventMsg, "expected reservation_failed event")

	var event app.ReservationFailedEvent
	err = json.Unmarshal(eventMsg.Data, &event)
	require.NoError(t, err)

	assert.Equal(t, "order-1", event.OrderID)
	assert.Equal(t, "prod-999", event.ProductID)
	assert.Equal(t, "product not found", event.Reason)
}

func TestStoreService_FullLifecycle(t *testing.T) {
	db, client, cleanup := setupTestInfra(t)
	defer cleanup()

	productRepo := mongoadapter.NewProductRepository(db)
	stockRepo := mongoadapter.NewStockRepository(db)
	eventPub := natsadapter.NewEventPublisher(client, itemsReservedSubj, reservationFailedSubj, orderConfirmedSubj, reservationReleasedSubj)
	storeSvc := app.NewStoreService(productRepo, stockRepo, eventPub, nil)

	worker := natsadapter.NewWorker(storeSvc)
	err := worker.SubscribeAll(context.Background(), client, reserveItemsSubj, confirmOrderSubj, releaseReservationSubj)
	require.NoError(t, err)

	// Step 1: Reserve items for order-1
	payload1, _ := json.Marshal(map[string]any{
		"user_id":    "user-1",
		"product_id": "prod-1",
		"quantity":   2,
	})
	msg1 := nats.NewMsg(reserveItemsSubj)
	msg1.Header.Set("X-Order-ID", "order-1")
	msg1.Data = payload1

	publishMsg(t, client, msg1)

	// Reserve items for order-2
	payload2, _ := json.Marshal(map[string]any{
		"user_id":    "user-2",
		"product_id": "prod-1",
		"quantity":   3,
	})
	msg2 := nats.NewMsg(reserveItemsSubj)
	msg2.Header.Set("X-Order-ID", "order-2")
	msg2.Data = payload2

	publishMsg(t, client, msg2)

	time.Sleep(500 * time.Millisecond)

	// Verify stock after reservations
	stock, err := stockRepo.Get(context.Background(), "prod-1")
	require.NoError(t, err)
	assert.Equal(t, 5, stock.Available, "10 - 2 - 3 = 5")
	assert.Equal(t, 5, stock.Reserved, "2 + 3 = 5")

	// Step 2: Confirm order-1
	confirmPayload, _ := json.Marshal(map[string]any{
		"order_id":   "order-1",
		"user_id":    "user-1",
		"product_id": "prod-1",
		"quantity":   2,
	})
	publishPayload(t, client, confirmOrderSubj, confirmPayload)

	time.Sleep(500 * time.Millisecond)

	// Verify stock after confirmation
	stock, err = stockRepo.Get(context.Background(), "prod-1")
	require.NoError(t, err)
	assert.Equal(t, 5, stock.Available, "unchanged")
	assert.Equal(t, 3, stock.Reserved, "5 - 2 = 3")

	// Step 3: Release reservation for order-2
	releasePayload, _ := json.Marshal(map[string]any{
		"order_id":   "order-2",
		"user_id":    "user-2",
		"product_id": "prod-1",
		"quantity":   3,
	})
	publishPayload(t, client, releaseReservationSubj, releasePayload)

	time.Sleep(500 * time.Millisecond)

	// Verify stock after release
	stock, err = stockRepo.Get(context.Background(), "prod-1")
	require.NoError(t, err)
	assert.Equal(t, 8, stock.Available, "5 + 3 = 8")
	assert.Equal(t, 0, stock.Reserved, "3 - 3 = 0")
}

func TestStoreService_ConcurrentReservations(t *testing.T) {
	db, client, cleanup := setupTestInfra(t)
	defer cleanup()

	productRepo := mongoadapter.NewProductRepository(db)
	stockRepo := mongoadapter.NewStockRepository(db)
	eventPub := natsadapter.NewEventPublisher(client, itemsReservedSubj, reservationFailedSubj, orderConfirmedSubj, reservationReleasedSubj)
	storeSvc := app.NewStoreService(productRepo, stockRepo, eventPub, nil)

	worker := natsadapter.NewWorker(storeSvc)
	err := worker.SubscribeAll(context.Background(), client, reserveItemsSubj, confirmOrderSubj, releaseReservationSubj)
	require.NoError(t, err)

	failedCh := jsSubscribe(t, client, reservationFailedSubj, "test-concurrent-failed")

	// Try to reserve more than available concurrently
	numRequests := 15
	quantity := 1

	for i := range numRequests {
		orderID := fmt.Sprintf("order-concurrent-%d", i)
		payload, _ := json.Marshal(map[string]any{
			"user_id":    "user-1",
			"product_id": "prod-1",
			"quantity":   quantity,
		})
		msg := nats.NewMsg(reserveItemsSubj)
		msg.Header.Set("X-Order-ID", orderID)
		msg.Data = payload

		publishMsg(t, client, msg)
	}

	time.Sleep(2 * time.Second)
	_ = failedCh

	// Verify total reserved <= available (10)
	stock, err := stockRepo.Get(context.Background(), "prod-1")
	require.NoError(t, err)
	assert.LessOrEqual(t, stock.Reserved, 10, "total reserved should not exceed available")
	assert.Equal(t, 10-stock.Reserved, stock.Available, "available + reserved should equal 10")
}

func TestSeedProducts_UUIDDemoCatalog(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	db, _, cleanup := setupTestInfra(t)
	defer cleanup()

	productColl := db.Collection("products")
	stockColl := db.Collection("stock")

	demoProducts := []struct {
		id    string
		name  string
		price int32
	}{
		{id: "22222222-2222-4222-8222-222222222222", name: "Demo Gadget", price: 2500},
		{id: "33333333-3333-4333-8333-333333333333", name: "USB Cable", price: 1500},
		{id: "44444444-4444-4444-8444-444444444444", name: "Phone Case", price: 3500},
	}

	for _, want := range demoProducts {
		var product struct {
			Name  string `bson:"name"`
			Price int32  `bson:"price"`
		}
		err := productColl.FindOne(ctx, bson.M{"_id": want.id}).Decode(&product)
		require.NoError(t, err, "product %s", want.id)
		assert.Equal(t, want.name, product.Name)
		assert.Equal(t, want.price, product.Price)

		var stock struct {
			Available int `bson:"available"`
		}
		err = stockColl.FindOne(ctx, bson.M{"product_id": want.id}).Decode(&stock)
		require.NoError(t, err, "stock %s", want.id)
		assert.Equal(t, 100, stock.Available)
	}

	require.NoError(t, mongoadapter.SeedProducts(ctx, db))

	for _, want := range demoProducts {
		var product struct {
			Name  string `bson:"name"`
			Price int32  `bson:"price"`
		}
		err := productColl.FindOne(ctx, bson.M{"_id": want.id}).Decode(&product)
		require.NoError(t, err, "product %s after reseed", want.id)
		assert.Equal(t, want.name, product.Name)
		assert.Equal(t, want.price, product.Price)
	}
}
