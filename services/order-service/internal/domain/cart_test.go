package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustOrderItem(t *testing.T, productID ProductID, quantity, unitPrice int64) OrderItem {
	t.Helper()

	item, err := NewOrderItem(productID, quantity, unitPrice)
	require.NoError(t, err)

	return *item
}

func TestNewCart(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	item := mustOrderItem(t, ProductID(uuid.New()), 1, 100)

	tests := []struct {
		name    string
		userID  UserID
		items   []OrderItem
		wantErr error
	}{
		{
			name:   "creates cart with items",
			userID: userID,
			items:  []OrderItem{item},
		},
		{
			name:   "creates empty cart when items is nil",
			userID: userID,
			items:  nil,
		},
		{
			name:    "requires userID",
			userID:  UserID{},
			items:   nil,
			wantErr: ErrUserIDRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cart, err := NewCart(tt.userID, tt.items...)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, cart)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, cart)
			assert.Equal(t, tt.userID, cart.UserID())
			assert.False(t, cart.UpdatedAt().IsZero())
			assert.Len(t, cart.Items(), len(tt.items))
		})
	}
}

func TestCart_AddItem(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	productA := ProductID(uuid.New())
	productB := ProductID(uuid.New())

	cart, err := NewCart(userID)
	require.NoError(t, err)

	itemA := mustOrderItem(t, productA, 2, 100)
	itemB := mustOrderItem(t, productB, 1, 50)

	require.NoError(t, cart.AddItem(itemA))
	require.NoError(t, cart.AddItem(itemB))
	assert.Len(t, cart.Items(), 2)

	addMoreA := mustOrderItem(t, productA, 3, 100)
	require.NoError(t, cart.AddItem(addMoreA))

	items := cart.Items()
	require.Len(t, items, 2)
	assert.Equal(t, int64(5), items[0].Quantity())
	assert.Equal(t, int64(500), items[0].TotalPrice())
}

func TestCart_AddItem_updatesUpdatedAt(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	cart, err := NewCart(userID)
	require.NoError(t, err)

	updatedAt := cart.UpdatedAt()
	time.Sleep(time.Millisecond)

	require.NoError(t, cart.AddItem(mustOrderItem(t, ProductID(uuid.New()), 1, 100)))
	assert.True(t, cart.UpdatedAt().After(updatedAt))
}

func TestCart_AddItem_rejectsUnitPriceMismatch(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	productID := ProductID(uuid.New())

	cart, err := NewCart(userID, mustOrderItem(t, productID, 1, 100))
	require.NoError(t, err)

	err = cart.AddItem(mustOrderItem(t, productID, 1, 200))
	require.ErrorIs(t, err, ErrUnitPriceMismatch)
	assert.Len(t, cart.Items(), 1)
}

func TestCart_RemoveItem(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	productA := ProductID(uuid.New())
	productB := ProductID(uuid.New())

	t.Run("removes existing item", func(t *testing.T) {
		t.Parallel()

		cart, err := NewCart(userID,
			mustOrderItem(t, productA, 1, 100),
			mustOrderItem(t, productB, 2, 50),
		)
		require.NoError(t, err)

		require.NoError(t, cart.RemoveItem(productA))
		assert.Len(t, cart.Items(), 1)
		assert.Equal(t, productB, cart.Items()[0].ProductID())
	})

	t.Run("returns error when item is missing", func(t *testing.T) {
		t.Parallel()

		c, err := NewCart(userID, mustOrderItem(t, productA, 1, 100))
		require.NoError(t, err)

		err = c.RemoveItem(ProductID(uuid.New()))
		require.ErrorIs(t, err, ErrItemNotFound)
	})
}

func TestCart_RemoveItem_updatesUpdatedAt(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	productID := ProductID(uuid.New())

	cart, err := NewCart(userID, mustOrderItem(t, productID, 1, 100))
	require.NoError(t, err)

	updatedAt := cart.UpdatedAt()
	time.Sleep(time.Millisecond)

	require.NoError(t, cart.RemoveItem(productID))
	assert.True(t, cart.UpdatedAt().After(updatedAt))
}

func TestCart_Items(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	items := []OrderItem{
		mustOrderItem(t, ProductID(uuid.New()), 1, 100),
		mustOrderItem(t, ProductID(uuid.New()), 2, 50),
	}

	cart, err := NewCart(userID, items...)
	require.NoError(t, err)
	assert.Equal(t, items, cart.Items())
}

func TestCart_Items_returnsClone(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	cart, err := NewCart(userID, mustOrderItem(t, ProductID(uuid.New()), 1, 100))
	require.NoError(t, err)

	items := cart.Items()
	items[0] = mustOrderItem(t, ProductID(uuid.New()), 99, 1)

	assert.Len(t, cart.Items(), 1)
	assert.Equal(t, int64(1), cart.Items()[0].Quantity())
}

func TestCart_TotalPrice(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	cart, err := NewCart(userID,
		mustOrderItem(t, ProductID(uuid.New()), 1, 100),
		mustOrderItem(t, ProductID(uuid.New()), 2, 50),
	)
	require.NoError(t, err)

	assert.Equal(t, int64(200), cart.TotalPrice())
}

func TestCart_Clear(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	cart, err := NewCart(userID, mustOrderItem(t, ProductID(uuid.New()), 1, 100))
	require.NoError(t, err)

	updatedAt := cart.UpdatedAt()
	time.Sleep(time.Millisecond)

	cart.Clear()

	assert.Empty(t, cart.Items())
	assert.True(t, cart.UpdatedAt().After(updatedAt))
}

func TestReconstituteCart(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	item := mustOrderItem(t, ProductID(uuid.New()), 1, 100)
	updatedAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	cart, err := ReconstituteCart(userID, updatedAt, item)
	require.NoError(t, err)
	require.NotNil(t, cart)
	assert.Equal(t, userID, cart.UserID())
	assert.Equal(t, updatedAt, cart.UpdatedAt())
	assert.Equal(t, []OrderItem{item}, cart.Items())
}

func TestCart_Checkout(t *testing.T) {
	t.Parallel()

	userID := UserID(uuid.New())
	orderID := OrderID(uuid.New())
	now := time.Now()

	t.Run("creates pending order from non-empty cart", func(t *testing.T) {
		t.Parallel()

		item := mustOrderItem(t, ProductID(uuid.New()), 1, 100)
		cart, err := NewCart(userID, item)
		require.NoError(t, err)

		order, err := cart.Checkout(orderID, "Moscow, Red Square 1", now)
		require.NoError(t, err)
		require.NotNil(t, order)
		assert.Equal(t, orderID, order.OrderID())
		assert.Equal(t, userID, order.UserID())
		assert.Equal(t, OrderStatusPending, order.Status())
		assert.Equal(t, PaymentID{}, order.PaymentID())
		assert.Equal(t, "Moscow, Red Square 1", order.DeliveryAddress())
		assert.Equal(t, int64(100), order.TotalPrice())
		assert.Equal(t, []OrderItem{item}, order.Items())
		assert.Equal(t, now, order.CreatedAt())
		assert.Equal(t, now, order.UpdatedAt())
	})

	t.Run("returns error for empty cart", func(t *testing.T) {
		t.Parallel()

		cart, err := NewCart(userID)
		require.NoError(t, err)

		order, err := cart.Checkout(orderID, "Moscow", now)
		require.ErrorIs(t, err, ErrEmptyCart)
		assert.Nil(t, order)
	})
}
