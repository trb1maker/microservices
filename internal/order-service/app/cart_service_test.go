package app

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/trb1maker/microservices/internal/order-service/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errRepoUnavailable = errors.New("repository unavailable")

type stubCartRepo struct {
	carts   map[domain.UserID]*domain.Cart
	getErr  error
	saveErr error
}

func newStubCartRepo() *stubCartRepo {
	return &stubCartRepo{carts: make(map[domain.UserID]*domain.Cart)}
}

func (r *stubCartRepo) Get(_ context.Context, userID domain.UserID) (*domain.Cart, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}

	cart, ok := r.carts[userID]
	if !ok {
		return nil, nil
	}

	return cart, nil
}

func (r *stubCartRepo) Save(_ context.Context, cart *domain.Cart) error {
	if r.saveErr != nil {
		return r.saveErr
	}

	r.carts[cart.UserID()] = cart

	return nil
}

func TestCartService_AddItem_createsCartWhenMissing(t *testing.T) {
	t.Parallel()

	repo := newStubCartRepo()
	service := NewCartService(repo)
	userID := domain.UserID(uuid.NewV7())

	item, err := domain.NewOrderItem(domain.ProductID(uuid.NewV7()), +2, 100)
	require.NoError(t, err)

	cart, err := service.AddItem(t.Context(), userID, *item)
	require.NoError(t, err)
	require.NotNil(t, cart)
	assert.Len(t, cart.Items(), 1)
	assert.Equal(t, int64(200), cart.TotalPrice())
}

func TestCartService_GetCart_createsEmptyCart(t *testing.T) {
	t.Parallel()

	repo := newStubCartRepo()
	service := NewCartService(repo)
	userID := domain.UserID(uuid.NewV7())

	cart, err := service.GetCart(t.Context(), userID)
	require.NoError(t, err)
	require.NotNil(t, cart)
	assert.Empty(t, cart.Items())
}

func TestCartService_RemoveItem_success(t *testing.T) {
	t.Parallel()

	repo := newStubCartRepo()
	service := NewCartService(repo)
	userID := domain.UserID(uuid.NewV7())
	productID := domain.ProductID(uuid.NewV7())

	item, err := domain.NewOrderItem(productID, 1, 100)
	require.NoError(t, err)

	cart, err := domain.NewCart(userID, *item)
	require.NoError(t, err)
	repo.carts[userID] = cart

	updated, err := service.RemoveItem(t.Context(), userID, productID)
	require.NoError(t, err)
	assert.Empty(t, updated.Items())
}

func TestCartService_RemoveItem_notFound(t *testing.T) {
	t.Parallel()

	repo := newStubCartRepo()
	service := NewCartService(repo)
	userID := domain.UserID(uuid.NewV7())

	_, err := service.RemoveItem(t.Context(), userID, domain.ProductID(uuid.NewV7()))
	require.ErrorIs(t, err, domain.ErrItemNotFound)
}

func TestCartService_getOrCreateCart_repoGetError(t *testing.T) {
	t.Parallel()

	repo := newStubCartRepo()
	repo.getErr = errRepoUnavailable
	service := NewCartService(repo)

	_, err := service.GetCart(t.Context(), domain.UserID(uuid.NewV7()))
	require.Error(t, err)
	assert.ErrorIs(t, err, errRepoUnavailable)
}

func TestCartService_HandleReservationFailed_clearedCartIsNoOp(t *testing.T) {
	t.Parallel()

	repo := newStubCartRepo()
	service := NewCartService(repo)
	userID := domain.UserID(uuid.NewV7())
	productID := domain.ProductID(uuid.NewV7())

	cart, err := domain.NewCart(userID, mustOrderItem(t, productID))
	require.NoError(t, err)
	cart.Clear()
	repo.carts[userID] = cart

	err = service.HandleReservationFailed(t.Context(), ReservationFailed{
		UserID:    uuid.UUID(userID).String(),
		ProductID: uuid.UUID(productID).String(),
	})
	require.NoError(t, err)
}

func mustOrderItem(t *testing.T, productID domain.ProductID) domain.OrderItem {
	t.Helper()
	item, err := domain.NewOrderItem(productID, 1, 100)
	require.NoError(t, err)
	return *item
}

func TestCartService_AddItem_repoSaveError(t *testing.T) {
	t.Parallel()

	repo := newStubCartRepo()
	repo.saveErr = errRepoUnavailable
	service := NewCartService(repo)
	userID := domain.UserID(uuid.NewV7())

	item, err := domain.NewOrderItem(domain.ProductID(uuid.NewV7()), 1, 100)
	require.NoError(t, err)

	_, err = service.AddItem(t.Context(), userID, *item)
	require.Error(t, err)
	assert.ErrorIs(t, err, errRepoUnavailable)
}
