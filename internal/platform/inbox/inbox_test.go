package inbox_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trb1maker/microservices/internal/platform/inbox"
)

func TestMemoryStore_SeenAfterMark(t *testing.T) {
	t.Parallel()

	store := inbox.NewMemoryStore()
	ctx := context.Background()

	seen, err := store.Seen(ctx, "order-saga", "evt-1")
	require.NoError(t, err)
	require.False(t, seen)

	require.NoError(t, store.Mark(ctx, "order-saga", "evt-1"))

	seen, err = store.Seen(ctx, "order-saga", "evt-1")
	require.NoError(t, err)
	require.True(t, seen)

	seen, err = store.Seen(ctx, "order-saga", "evt-2")
	require.NoError(t, err)
	require.False(t, seen)
}
