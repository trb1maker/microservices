package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cartmemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/cart_repository/memory"
	ordermemory "github.com/trb1maker/microservices/services/order-service/internal/adapters/order_repository/memory"
	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingEventPublisher struct {
	mu sync.Mutex

	orderCreatedErr       error
	orderCancelledErr     error
	releaseReservationErr error

	orderCreated       []OrderCreated
	orderCancelled     []OrderCancelled
	releaseReservation []ReleaseReservation
}

func (p *recordingEventPublisher) PublishReserveItems(context.Context, ReserveItems) error {
	return nil
}

func (p *recordingEventPublisher) PublishReleaseReservation(_ context.Context, event ReleaseReservation) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.releaseReservation = append(p.releaseReservation, event)

	return p.releaseReservationErr
}

func (p *recordingEventPublisher) PublishOrderCreated(_ context.Context, event OrderCreated) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.orderCreated = append(p.orderCreated, event)

	return p.orderCreatedErr
}

func (p *recordingEventPublisher) PublishConfirmOrder(context.Context, ConfirmOrder) error {
	return nil
}

func (p *recordingEventPublisher) PublishOrderFinalized(context.Context, OrderFinalized) error {
	return nil
}

func (p *recordingEventPublisher) PublishOrderCancelled(_ context.Context, event OrderCancelled) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.orderCancelled = append(p.orderCancelled, event)

	return p.orderCancelledErr
}

func setupOrderService(t *testing.T, events EventPublisher) (*OrderService, domain.UserID) {
	t.Helper()

	cartRepo := cartmemory.NewCartRepository()
	orderRepo := ordermemory.NewOrderRepository()
	userID := domain.UserID(uuid.New())

	item, err := domain.NewOrderItem(domain.ProductID(uuid.New()), 1, 100)
	require.NoError(t, err)

	cart, err := domain.NewCart(userID, *item)
	require.NoError(t, err)
	require.NoError(t, cartRepo.Save(t.Context(), cart))

	return NewOrderService(cartRepo, orderRepo, events), userID
}

func TestOrderService_Checkout_happyPath(t *testing.T) {
	t.Parallel()

	events := &recordingEventPublisher{}
	service, userID := setupOrderService(t, events)
	now := time.Now()

	order, err := service.Checkout(t.Context(), userID, "Moscow", now)
	require.NoError(t, err)
	require.NotNil(t, order)
	assert.Equal(t, "Moscow", order.DeliveryAddress())
	assert.Equal(t, domain.OrderStatusPending, order.Status())

	cart, err := service.carts.Get(t.Context(), userID)
	require.NoError(t, err)
	require.NotNil(t, cart)
	assert.Empty(t, cart.Items())

	events.mu.Lock()
	defer events.mu.Unlock()
	require.Len(t, events.orderCreated, 1)
	assert.Equal(t, uuid.UUID(order.OrderID()).String(), events.orderCreated[0].OrderID)
}

var errPublishUnavailable = errors.New("publish unavailable")

func TestOrderService_Checkout_publishFailureRollsBack(t *testing.T) {
	t.Parallel()

	events := &recordingEventPublisher{orderCreatedErr: errPublishUnavailable}
	service, userID := setupOrderService(t, events)

	order, err := service.Checkout(t.Context(), userID, "Moscow", time.Now())
	require.Error(t, err)
	assert.Nil(t, order)

	cart, err := service.carts.Get(t.Context(), userID)
	require.NoError(t, err)
	require.NotNil(t, cart)
	assert.Len(t, cart.Items(), 1)
}

func TestOrderService_GetOrder_wrongUser(t *testing.T) {
	t.Parallel()

	service, userID := setupOrderService(t, NewNoopEventPublisher())

	order, err := service.Checkout(t.Context(), userID, "Moscow", time.Now())
	require.NoError(t, err)

	otherUser := domain.UserID(uuid.New())
	got, err := service.GetOrder(t.Context(), otherUser, order.OrderID())
	require.ErrorIs(t, err, ErrOrderNotFound)
	assert.Nil(t, got)
}

func TestOrderService_CancelOrder_happyPath(t *testing.T) {
	t.Parallel()

	events := &recordingEventPublisher{}
	service, userID := setupOrderService(t, events)

	order, err := service.Checkout(t.Context(), userID, "Moscow", time.Now())
	require.NoError(t, err)

	cancelled, err := service.CancelOrder(t.Context(), userID, order.OrderID(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, domain.OrderStatusCancelled, cancelled.Status())

	events.mu.Lock()
	defer events.mu.Unlock()
	require.Len(t, events.orderCancelled, 1)
	require.Len(t, events.releaseReservation, 1)
}

func TestOrderService_CancelOrder_wrongUser(t *testing.T) {
	t.Parallel()

	service, userID := setupOrderService(t, NewNoopEventPublisher())

	order, err := service.Checkout(t.Context(), userID, "Moscow", time.Now())
	require.NoError(t, err)

	otherUser := domain.UserID(uuid.New())
	cancelled, err := service.CancelOrder(t.Context(), otherUser, order.OrderID(), time.Now())
	require.ErrorIs(t, err, ErrOrderNotFound)
	assert.Nil(t, cancelled)
}
