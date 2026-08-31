package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trb1maker/microservices/internal/store-service/domain"
)

const (
	testProductID = "prod-1"
	testOrderID   = "order-1"
	testUserID    = "user-1"
)

var (
	errTestDB        = errors.New("db error")
	errTestRedisConn = errors.New("connection refused")
)

type mockProductRepo struct {
	products map[string]*domain.Product
}

func newMockProductRepo() *mockProductRepo {
	return &mockProductRepo{
		products: map[string]*domain.Product{
			testProductID: {ID: testProductID, Name: "Test Product", Price: 1000},
		},
	}
}

func (m *mockProductRepo) Get(_ context.Context, id domain.ProductID) (*domain.Product, error) {
	product, ok := m.products[string(id)]
	if !ok {
		return nil, domain.ErrProductNotFound
	}
	return product, nil
}

type mockStockRepo struct {
	stocks    map[string]*domain.Stock
	updateErr error
}

func newMockStockRepo() *mockStockRepo {
	return &mockStockRepo{
		stocks: map[string]*domain.Stock{
			testProductID: {ProductID: testProductID, Available: 10, Reserved: 0},
		},
	}
}

func (m *mockStockRepo) Get(_ context.Context, productID domain.ProductID) (*domain.Stock, error) {
	stock, ok := m.stocks[string(productID)]
	if !ok {
		return nil, domain.ErrStockNotFound
	}
	return stock, nil
}

func (m *mockStockRepo) Update(_ context.Context, stock *domain.Stock) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.stocks[string(stock.ProductID)] = stock
	return nil
}

type mockEventPublisher struct {
	itemsReserved       []ItemsReservedEvent
	reservationFailed   []ReservationFailedEvent
	orderConfirmed      []OrderConfirmedEvent
	reservationReleased []ReservationReleasedEvent
}

func newMockEventPublisher() *mockEventPublisher {
	return &mockEventPublisher{}
}

func (m *mockEventPublisher) PublishItemsReserved(_ context.Context, event ItemsReservedEvent) error {
	m.itemsReserved = append(m.itemsReserved, event)
	return nil
}

func (m *mockEventPublisher) PublishReservationFailed(_ context.Context, event ReservationFailedEvent) error {
	m.reservationFailed = append(m.reservationFailed, event)
	return nil
}

func (m *mockEventPublisher) PublishOrderConfirmed(_ context.Context, event OrderConfirmedEvent) error {
	m.orderConfirmed = append(m.orderConfirmed, event)
	return nil
}

func (m *mockEventPublisher) PublishReservationReleased(_ context.Context, event ReservationReleasedEvent) error {
	m.reservationReleased = append(m.reservationReleased, event)
	return nil
}

func TestReserveItems_Success(t *testing.T) {
	products := newMockProductRepo()
	stocks := newMockStockRepo()
	events := newMockEventPublisher()
	svc := NewStoreService(products, stocks, events, nil)

	err := svc.ReserveItems(context.Background(), ReserveItemsRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.NoError(t, err)

	stock := stocks.stocks[testProductID]
	assert.Equal(t, 8, stock.Available)
	assert.Equal(t, 2, stock.Reserved)

	require.Len(t, events.itemsReserved, 1)
	assert.Equal(t, testOrderID, events.itemsReserved[0].OrderID)
	assert.Equal(t, testProductID, events.itemsReserved[0].ProductID)
	assert.Equal(t, 2, events.itemsReserved[0].Quantity)
}

func TestReserveItems_InvalidQuantity(t *testing.T) {
	svc := NewStoreService(newMockProductRepo(), newMockStockRepo(), newMockEventPublisher(), nil)

	err := svc.ReserveItems(context.Background(), ReserveItemsRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  0,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid quantity")
}

func TestReserveItems_ProductNotFound(t *testing.T) {
	products := newMockProductRepo()
	delete(products.products, testProductID)
	events := newMockEventPublisher()
	svc := NewStoreService(products, newMockStockRepo(), events, nil)

	err := svc.ReserveItems(context.Background(), ReserveItemsRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrProductNotFound)

	require.Len(t, events.reservationFailed, 1)
	assert.Equal(t, "product not found", events.reservationFailed[0].Reason)
}

func TestReserveItems_StockNotFound(t *testing.T) {
	stocks := newMockStockRepo()
	delete(stocks.stocks, testProductID)
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), stocks, events, nil)

	err := svc.ReserveItems(context.Background(), ReserveItemsRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrStockNotFound)

	require.Len(t, events.reservationFailed, 1)
	assert.Equal(t, "stock not found", events.reservationFailed[0].Reason)
}

func TestReserveItems_InsufficientStock(t *testing.T) {
	stocks := newMockStockRepo()
	stocks.stocks[testProductID].Available = 1
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), stocks, events, nil)

	err := svc.ReserveItems(context.Background(), ReserveItemsRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  5,
	})

	require.NoError(t, err)

	require.Len(t, events.reservationFailed, 1)
	assert.Equal(t, "insufficient stock", events.reservationFailed[0].Reason)
}

func TestReserveItems_UpdateError(t *testing.T) {
	stocks := newMockStockRepo()
	stocks.updateErr = errTestDB
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), stocks, events, nil)

	err := svc.ReserveItems(context.Background(), ReserveItemsRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")

	require.Len(t, events.reservationFailed, 1)
	assert.Equal(t, "update failed", events.reservationFailed[0].Reason)
}

func TestConfirmOrder_Success(t *testing.T) {
	stocks := newMockStockRepo()
	stocks.stocks[testProductID].Available = 8
	stocks.stocks[testProductID].Reserved = 2
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), stocks, events, nil)

	err := svc.ConfirmOrder(context.Background(), ConfirmOrderRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.NoError(t, err)

	stock := stocks.stocks[testProductID]
	assert.Equal(t, 8, stock.Available)
	assert.Equal(t, 0, stock.Reserved)

	require.Len(t, events.orderConfirmed, 1)
	assert.Equal(t, testOrderID, events.orderConfirmed[0].OrderID)
	assert.Equal(t, testUserID, events.orderConfirmed[0].UserID)
}

func TestConfirmOrder_InsufficientReserved(t *testing.T) {
	stocks := newMockStockRepo()
	stocks.stocks[testProductID].Available = 8
	stocks.stocks[testProductID].Reserved = 1
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), stocks, events, nil)

	err := svc.ConfirmOrder(context.Background(), ConfirmOrderRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient reserved stock")

	require.Len(t, events.reservationFailed, 1)
	assert.Equal(t, "reservation not found", events.reservationFailed[0].Reason)
}

func TestReleaseReservation_Success(t *testing.T) {
	stocks := newMockStockRepo()
	stocks.stocks[testProductID].Available = 8
	stocks.stocks[testProductID].Reserved = 2
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), stocks, events, nil)

	err := svc.ReleaseReservation(context.Background(), ReleaseReservationRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.NoError(t, err)

	stock := stocks.stocks[testProductID]
	assert.Equal(t, 10, stock.Available)
	assert.Equal(t, 0, stock.Reserved)

	require.Len(t, events.reservationReleased, 1)
	assert.Equal(t, testOrderID, events.reservationReleased[0].OrderID)
	assert.Equal(t, testProductID, events.reservationReleased[0].ProductID)
	assert.Equal(t, 2, events.reservationReleased[0].Quantity)
}

type failingLocker struct{}

func (failingLocker) WithLock(context.Context, string, func(context.Context) error) error {
	return errLockNotAcquired
}

type redisErrorLocker struct{}

func (redisErrorLocker) WithLock(context.Context, string, func(context.Context) error) error {
	return fmt.Errorf("acquire lock: %w", errTestRedisConn)
}

func TestReserveItems_LockNotAcquired(t *testing.T) {
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), newMockStockRepo(), events, failingLocker{})

	err := svc.ReserveItems(context.Background(), ReserveItemsRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  1,
	})

	require.NoError(t, err)
	require.Len(t, events.reservationFailed, 1)
	assert.Equal(t, "lock not acquired", events.reservationFailed[0].Reason)
}

func TestReserveItems_LockUnavailable(t *testing.T) {
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), newMockStockRepo(), events, redisErrorLocker{})

	err := svc.ReserveItems(context.Background(), ReserveItemsRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  1,
	})

	require.NoError(t, err)
	require.Len(t, events.reservationFailed, 1)
	assert.Equal(t, "lock unavailable", events.reservationFailed[0].Reason)
}

func TestConfirmOrder_LockNotAcquired(t *testing.T) {
	stocks := newMockStockRepo()
	stocks.stocks[testProductID].Available = 8
	stocks.stocks[testProductID].Reserved = 2
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), stocks, events, failingLocker{})

	err := svc.ConfirmOrder(context.Background(), ConfirmOrderRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, errLockNotAcquired)
	require.Empty(t, events.orderConfirmed)
}

func TestReleaseReservation_LockNotAcquired(t *testing.T) {
	stocks := newMockStockRepo()
	stocks.stocks[testProductID].Available = 8
	stocks.stocks[testProductID].Reserved = 2
	events := newMockEventPublisher()
	svc := NewStoreService(newMockProductRepo(), stocks, events, failingLocker{})

	err := svc.ReleaseReservation(context.Background(), ReleaseReservationRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, errLockNotAcquired)
	require.Empty(t, events.reservationReleased)
}

func TestReleaseReservation_InsufficientReserved(t *testing.T) {
	stocks := newMockStockRepo()
	stocks.stocks[testProductID].Available = 8
	stocks.stocks[testProductID].Reserved = 1
	svc := NewStoreService(newMockProductRepo(), stocks, nil, nil)

	err := svc.ReleaseReservation(context.Background(), ReleaseReservationRequest{
		OrderID:   testOrderID,
		UserID:    testUserID,
		ProductID: testProductID,
		Quantity:  2,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient reserved stock")
}

var (
	_ ProductRepository = (*mockProductRepo)(nil)
	_ StockRepository   = (*mockStockRepo)(nil)
	_ EventPublisher    = (*mockEventPublisher)(nil)
)
