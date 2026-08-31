package app

import "context"

// StockLocker serializes stock mutations for a product.
type StockLocker interface {
	WithLock(ctx context.Context, productID string, fn func(context.Context) error) error
}

// NoopStockLocker runs the callback without locking (tests and local dev).
type NoopStockLocker struct{}

func (NoopStockLocker) WithLock(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}
