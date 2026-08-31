package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/trb1maker/microservices/internal/platform/redisx"
	"github.com/trb1maker/microservices/internal/store-service/app"
)

const stockLockPrefix = "lock:stock:"

// StockLocker implements app.StockLocker using Redis.
type StockLocker struct {
	mutex *redisx.Mutex
}

type StockLockerConfig struct {
	TTL        time.Duration
	RetryCount int
	RetryDelay time.Duration
}

func NewStockLocker(client *goredis.Client, cfg StockLockerConfig) *StockLocker {
	return &StockLocker{
		mutex: redisx.NewMutex(client, redisx.MutexConfig{
			KeyPrefix:  stockLockPrefix,
			TTL:        cfg.TTL,
			RetryCount: cfg.RetryCount,
			RetryDelay: cfg.RetryDelay,
		}),
	}
}

func (l *StockLocker) WithLock(ctx context.Context, productID string, fn func(context.Context) error) error {
	if err := l.mutex.WithLock(ctx, productID, fn); err != nil {
		return fmt.Errorf("stock lock: %w", err)
	}

	return nil
}

var _ app.StockLocker = (*StockLocker)(nil)
