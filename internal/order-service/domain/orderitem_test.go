package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrderItem(t *testing.T) {
	t.Parallel()

	validProductID := ProductID(uuid.New())

	tests := []struct {
		name      string
		productID ProductID
		quantity  int64
		unitPrice int64
		wantErr   error
	}{
		{
			name:      "valid item",
			productID: validProductID,
			quantity:  2,
			unitPrice: 100,
		},
		{
			name:      "empty productID",
			productID: ProductID{},
			quantity:  1,
			unitPrice: 100,
			wantErr:   ErrNotValidOrderItem,
		},
		{
			name:      "zero quantity",
			productID: validProductID,
			quantity:  0,
			unitPrice: 100,
			wantErr:   ErrNotValidOrderItem,
		},
		{
			name:      "negative quantity",
			productID: validProductID,
			quantity:  -1,
			unitPrice: 100,
			wantErr:   ErrNotValidOrderItem,
		},
		{
			name:      "zero unit price",
			productID: validProductID,
			quantity:  1,
			unitPrice: 0,
			wantErr:   ErrNotValidOrderItem,
		},
		{
			name:      "negative unit price",
			productID: validProductID,
			quantity:  1,
			unitPrice: -10,
			wantErr:   ErrNotValidOrderItem,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			item, err := NewOrderItem(tt.productID, tt.quantity, tt.unitPrice)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, item)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, item)
			assert.Equal(t, tt.productID, item.ProductID())
			assert.Equal(t, tt.quantity, item.Quantity())
			assert.Equal(t, tt.unitPrice, item.UnitPrice())
			assert.Equal(t, tt.quantity*tt.unitPrice, item.TotalPrice())
		})
	}
}

func TestOrderItem_Merge(t *testing.T) {
	t.Parallel()

	productID := ProductID(uuid.New())

	base, err := NewOrderItem(productID, 2, 100)
	require.NoError(t, err)

	addition, err := NewOrderItem(productID, 3, 100)
	require.NoError(t, err)

	otherProduct, err := NewOrderItem(ProductID(uuid.New()), 1, 50)
	require.NoError(t, err)

	differentPrice, err := NewOrderItem(productID, 1, 200)
	require.NoError(t, err)

	t.Run("merges quantities and totals for same product", func(t *testing.T) {
		t.Parallel()

		item := *base
		merged, err := item.Merge(*addition)
		require.NoError(t, err)
		assert.Equal(t, int64(5), merged.Quantity())
		assert.Equal(t, int64(500), merged.TotalPrice())
		assert.Equal(t, int64(2), item.Quantity())
		assert.Equal(t, int64(200), item.TotalPrice())
	})

	t.Run("rejects productID mismatch", func(t *testing.T) {
		t.Parallel()

		item := *base
		merged, err := item.Merge(*otherProduct)
		require.ErrorIs(t, err, ErrProductIDMismatch)
		assert.Equal(t, OrderItem{}, merged)
	})

	t.Run("rejects unitPrice mismatch", func(t *testing.T) {
		t.Parallel()

		item := *base
		merged, err := item.Merge(*differentPrice)
		require.ErrorIs(t, err, ErrUnitPriceMismatch)
		assert.Equal(t, OrderItem{}, merged)
	})
}
