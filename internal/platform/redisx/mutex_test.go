//go:build integration

package redisx_test

import (
	"context"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/trb1maker/microservices/internal/platform/redisx"
)

func TestMutex_WithLock_serializesAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:8.8-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	opts, err := goredis.ParseURL(connStr)
	require.NoError(t, err)

	client := goredis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })

	mutex := redisx.NewMutex(client, redisx.MutexConfig{
		KeyPrefix:  "test-lock:",
		TTL:        time.Second,
		RetryCount: 20,
		RetryDelay: 25 * time.Millisecond,
	})

	var counter int
	var wg sync.WaitGroup

	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := mutex.WithLock(ctx, "counter", func(context.Context) error {
				current := counter
				time.Sleep(10 * time.Millisecond)
				counter = current + 1
				return nil
			})
			require.NoError(t, err)
		}()
	}

	wg.Wait()
	assert.Equal(t, 5, counter)
}

func TestMutex_LockNotAcquired_withoutRetry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	container, err := redis.Run(ctx, "redis:8.8-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	opts, err := goredis.ParseURL(connStr)
	require.NoError(t, err)

	client := goredis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })

	mutex := redisx.NewMutex(client, redisx.MutexConfig{
		KeyPrefix:  "test-lock:",
		TTL:        time.Second,
		RetryCount: 0,
	})

	token, err := mutex.Lock(ctx, "held")
	require.NoError(t, err)

	_, err = mutex.Lock(ctx, "held")
	require.ErrorIs(t, err, redisx.ErrLockNotAcquired)

	require.NoError(t, mutex.Unlock(ctx, "held", token))
}
