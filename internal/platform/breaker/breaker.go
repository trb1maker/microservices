package breaker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker/v2"
)

var ErrOpen = errors.New("circuit breaker is open")

type Config struct {
	Name        string
	MaxFailures uint32
	OpenTimeout time.Duration
	HalfOpenMax uint32
}

const (
	defaultMaxFailures = 5
	defaultOpenTimeout = 10 * time.Second
	defaultHalfOpenMax = 1
)

type Breaker struct {
	cb *gobreaker.CircuitBreaker[struct{}]
}

func New(cfg Config) *Breaker {
	if cfg.Name == "" {
		cfg.Name = "default"
	}
	if cfg.MaxFailures == 0 {
		cfg.MaxFailures = defaultMaxFailures
	}
	if cfg.OpenTimeout == 0 {
		cfg.OpenTimeout = defaultOpenTimeout
	}
	if cfg.HalfOpenMax == 0 {
		cfg.HalfOpenMax = defaultHalfOpenMax
	}
	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.HalfOpenMax,
		Interval:    0,
		Timeout:     cfg.OpenTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.MaxFailures
		},
	}
	return &Breaker{cb: gobreaker.NewCircuitBreaker[struct{}](settings)}
}

func (b *Breaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	_, err := b.cb.Execute(func() (struct{}, error) {
		if err := ctx.Err(); err != nil {
			return struct{}{}, fmt.Errorf("context: %w", err)
		}
		if err := fn(ctx); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return fmt.Errorf("%w: %w", ErrOpen, err)
	}
	return fmt.Errorf("execute: %w", err)
}
