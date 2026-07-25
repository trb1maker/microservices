package nats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	cartmemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/cart_repository/memory"
	natsadapter "github.com/trb1maker/microservices/services/order-service/internal/adapters/event_consumer/nats"
	ordermemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/order_repository/memory"
	"github.com/trb1maker/microservices/services/order-service/internal/app"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	"github.com/google/uuid"
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

	natsContainer, err := tcnats.Run(ctx, "nats:2.14-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })

	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	nc, err := nats.Connect(natsURL)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	cartRepo := cartmemory.NewCartRepository()
	orderRepo := ordermemory.NewOrderRepository()
	events := app.NewNoopEventPublisher()
	carts := app.NewCartService(cartRepo, events)
	orders := app.NewOrderService(cartRepo, orderRepo, events, app.NewNoopPaymentClient(), app.NewNoopOrderMetrics())

	userID := domain.UserID(uuid.New())
	productID := domain.ProductID(uuid.New())
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
		domain.PaymentID(uuid.New()),
		"Moscow",
		time.Now().UTC(),
		time.Now().UTC(),
		*item,
	)
	require.NoError(t, err)
	require.NoError(t, orderRepo.Save(context.Background(), paidOrder))

	consumer := natsadapter.NewConsumer(nc, natsadapter.Subjects{
		ItemsReserved:     itemsReservedTestSubject,
		ReservationFailed: reservationFailedTestSubject,
		OrderConfirmed:    orderConfirmedTestSubject,
	}, carts, orders)
	require.NoError(t, consumer.Start())
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
	require.NoError(t, nc.Publish(reservationFailedTestSubject, payload))
	require.NoError(t, nc.Flush())

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
