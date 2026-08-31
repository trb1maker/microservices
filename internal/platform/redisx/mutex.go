package redisx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrLockNotAcquired = errors.New("lock not acquired")
	ErrLockNotHeld     = errors.New("lock not held")
)

const unlockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`

// Mutex implements a Redis-based distributed lock.
type Mutex struct {
	client     *goredis.Client
	keyPrefix  string
	ttl        time.Duration
	retryCount int
	retryDelay time.Duration
}

const (
	defaultMutexTTL        = 5 * time.Second
	defaultMutexRetryDelay = 50 * time.Millisecond
)

type MutexConfig struct {
	KeyPrefix  string
	TTL        time.Duration
	RetryCount int
	RetryDelay time.Duration
}

func NewMutex(client *goredis.Client, cfg MutexConfig) *Mutex {
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "lock:"
	}
	if cfg.TTL <= 0 {
		cfg.TTL = defaultMutexTTL
	}
	if cfg.RetryCount < 0 {
		cfg.RetryCount = 0
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = defaultMutexRetryDelay
	}

	return &Mutex{
		client:     client,
		keyPrefix:  cfg.KeyPrefix,
		ttl:        cfg.TTL,
		retryCount: cfg.RetryCount,
		retryDelay: cfg.RetryDelay,
	}
}

func (m *Mutex) Lock(ctx context.Context, key string) (token string, err error) {
	fullKey := m.keyPrefix + key
	token = uuid.NewString()

	for attempt := 0; attempt <= m.retryCount; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, m.retryDelay); err != nil {
				return "", err
			}
		}

		ok, err := m.client.SetNX(ctx, fullKey, token, m.ttl).Result()
		if err != nil {
			return "", fmt.Errorf("acquire lock: %w", err)
		}
		if ok {
			return token, nil
		}
	}

	return "", ErrLockNotAcquired
}

func (m *Mutex) Unlock(ctx context.Context, key, token string) error {
	fullKey := m.keyPrefix + key

	result, err := m.client.Eval(ctx, unlockScript, []string{fullKey}, token).Int64()
	if err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	if result == 0 {
		return ErrLockNotHeld
	}

	return nil
}

func (m *Mutex) WithLock(ctx context.Context, key string, fn func(context.Context) error) error {
	token, err := m.Lock(ctx, key)
	if err != nil {
		return err
	}

	defer func() {
		if unlockErr := m.Unlock(context.WithoutCancel(ctx), key, token); unlockErr != nil {
			_ = unlockErr
		}
	}()

	return fn(ctx)
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for lock: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
