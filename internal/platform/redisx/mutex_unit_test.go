package redisx_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trb1maker/microservices/internal/platform/redisx"
)

func TestMutex_LockUnlock_miniredis(t *testing.T) {
	t.Parallel()

	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	client := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	mutex := redisx.NewMutex(client, redisx.MutexConfig{TTL: time.Second})

	token, err := mutex.Lock(context.Background(), "item-1")
	require.NoError(t, err)

	_, err = mutex.Lock(context.Background(), "item-1")
	require.ErrorIs(t, err, redisx.ErrLockNotAcquired)

	require.NoError(t, mutex.Unlock(context.Background(), "item-1", token))

	token2, err := mutex.Lock(context.Background(), "item-1")
	require.NoError(t, err)
	assert.NotEmpty(t, token2)
}
