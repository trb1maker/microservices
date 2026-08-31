//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	cartmemory "github.com/trb1maker/microservices/internal/order-service/adapters/cart_repository/memory"
	natsadapter "github.com/trb1maker/microservices/internal/order-service/adapters/event_consumer/nats"
	ordermemory "github.com/trb1maker/microservices/internal/order-service/adapters/order_repository/memory"
	"github.com/trb1maker/microservices/internal/order-service/app"
	"github.com/trb1maker/microservices/internal/order-service/domain"
	"github.com/trb1maker/microservices/internal/platform/inbox"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	"github.com/trb1maker/microservices/tests/internal/natstest"
)

const (
	reservationFailedTestSubject = "store.reservation_failed.test"
	itemsReservedTestSubject     = "store.items_reserved.test"
	orderConfirmedTestSubject    = "store.order_confirmed.test"
)

func TestConsumer_handleReservationFailed_routesToCartAndOrder(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	consumerCtx := context.WithoutCancel(ctx)

	natsContainer, err := tcnats.Run(ctx, natstest.Image, natstest.ContainerOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })

	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	client := natstest.NewClient(t, natsURL)
	t.Cleanup(client.Conn().Close)

	cartRepo := cartmemory.NewCartRepository()
	orderRepo := ordermemory.NewOrderRepository()
	events := app.NewNoopEventPublisher()
	carts := app.NewCartService(cartRepo, events)
	orders := app.NewOrderService(cartRepo, orderRepo, events, app.NewNoopPaymentClient(), app.NewNoopOrderMetrics())

	userID := domain.UserID(uuid.NewV7())
	productID := domain.ProductID(uuid.NewV7())
	item, err := domain.NewOrderItem(productID, 1, 100)
	require.NoError(t, err)
	cart, err := domain.NewCart(userID, *item)
	require.NoError(t, err)
	cart.EnsurePendingOrderID()
	require.NoError(t, cartRepo.Save(context.Background(), cart))

	orderID := cart.PendingOrderID()
	paidOrder, err := domain.NewOrder(
		orderID,
		userID,
		domain.OrderStatusPaid,
		domain.PaymentID(uuid.NewV7()),
		"Moscow",
		time.Now().UTC(),
		time.Now().UTC(),
		*item,
	)
	require.NoError(t, err)
	require.NoError(t, orderRepo.Save(context.Background(), paidOrder))

	consumer := natsadapter.NewConsumer(client, natsadapter.Subjects{
		ItemsReserved:     itemsReservedTestSubject,
		ReservationFailed: reservationFailedTestSubject,
		OrderConfirmed:    orderConfirmedTestSubject,
	}, carts, orders)
	require.NoError(t, consumer.Start(consumerCtx))
	t.Cleanup(consumer.Close)

	event := app.ReservationFailed{
		OrderID:   uuid.UUID(orderID).String(),
		UserID:    uuid.UUID(userID).String(),
		ProductID: uuid.UUID(productID).String(),
		Quantity:  1,
		Reason:    "reservation not found",
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, client.Publish(ctx, reservationFailedTestSubject, payload))

	require.Eventually(t, func() bool {
		current, getErr := orderRepo.Get(context.Background(), orderID)
		return getErr == nil && current != nil && current.Status() == domain.OrderStatusCancelled
	}, 2*time.Second, 50*time.Millisecond)

	updatedCart, err := cartRepo.Get(context.Background(), userID)
	require.NoError(t, err)
	require.NotNil(t, updatedCart)
	require.Len(t, updatedCart.Items(), 1)
	assert.Equal(t, domain.ReservationStatusFailed, updatedCart.Items()[0].ReservationStatus())
}

type countingPublisher struct {
	app.NoopEventPublisher
	mu        sync.Mutex
	finalized int
}

func (p *countingPublisher) PublishOrderFinalized(context.Context, app.OrderFinalized) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finalized++
	return nil
}

func (p *countingPublisher) finalizedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.finalized
}

func TestConsumer_duplicateMsgIDDoesNotFinalizeTwice(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	consumerCtx := context.WithoutCancel(ctx)

	natsContainer, err := tcnats.Run(ctx, natstest.Image, natstest.ContainerOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })

	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)
	client := natstest.NewClient(t, natsURL)
	t.Cleanup(client.Conn().Close)

	cartRepo := cartmemory.NewCartRepository()
	orderRepo := ordermemory.NewOrderRepository()
	events := &countingPublisher{}
	carts := app.NewCartService(cartRepo, events)
	orders := app.NewOrderService(cartRepo, orderRepo, events, app.NewNoopPaymentClient(), app.NewNoopOrderMetrics())

	userID := domain.UserID(uuid.NewV7())
	productID := domain.ProductID(uuid.NewV7())
	item, err := domain.NewOrderItem(productID, 1, 100)
	require.NoError(t, err)
	orderID := domain.OrderID(uuid.NewV7())
	paidOrder, err := domain.NewOrder(
		orderID,
		userID,
		domain.OrderStatusPaid,
		domain.PaymentID(uuid.NewV7()),
		"Moscow",
		time.Now().UTC(),
		time.Now().UTC(),
		*item,
	)
	require.NoError(t, err)
	require.NoError(t, orderRepo.Save(context.Background(), paidOrder))

	consumer := natsadapter.NewConsumer(client, natsadapter.Subjects{
		ItemsReserved:     itemsReservedTestSubject + ".inbox",
		ReservationFailed: reservationFailedTestSubject + ".inbox",
		OrderConfirmed:    orderConfirmedTestSubject + ".inbox",
	}, carts, orders)
	consumer.SetInbox(inbox.ForConsumer(inbox.NewMemoryStore(), "order-saga"))
	require.NoError(t, consumer.Start(consumerCtx))
	t.Cleanup(consumer.Close)

	event := app.OrderConfirmed{
		OrderID:   uuid.UUID(orderID).String(),
		UserID:    uuid.UUID(userID).String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	headers := nats.Header{}
	headers.Set(natsx.HeaderMsgID, uuid.NewV7().String())
	require.NoError(t, client.PublishJSON(ctx, orderConfirmedTestSubject+".inbox", event, headers))
	require.NoError(t, client.PublishJSON(ctx, orderConfirmedTestSubject+".inbox", event, headers))

	require.Eventually(t, func() bool {
		current, getErr := orderRepo.Get(context.Background(), orderID)
		return getErr == nil && current != nil && current.Status() == domain.OrderStatusConfirmed && events.finalizedCount() == 1
	}, 2*time.Second, 50*time.Millisecond)

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, 1, events.finalizedCount())
}
