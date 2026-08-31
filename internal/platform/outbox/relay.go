package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type RelayConfig struct {
	PollInterval time.Duration
	BatchSize    int
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	Lease        time.Duration
}

const (
	defaultPollInterval = 500 * time.Millisecond
	defaultBatchSize    = 50
	defaultBaseBackoff  = time.Second
	defaultMaxBackoff   = time.Minute
	defaultLease        = 2 * time.Second
)

type Relay struct {
	store     Store
	publisher Publisher
	cfg       RelayConfig
}

func NewRelay(store Store, publisher Publisher, cfg RelayConfig) *Relay {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = defaultBaseBackoff
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = defaultMaxBackoff
	}
	if cfg.Lease <= 0 {
		cfg.Lease = defaultLease
	}
	return &Relay{store: store, publisher: publisher, cfg: cfg}
}

func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("outbox relay stopped: %w", ctx.Err())
		case <-ticker.C:
			if err := r.processBatch(ctx); err != nil {
				slog.Error("outbox relay batch failed", slog.Any("error", err))
			}
		}
	}
}

func (r *Relay) processBatch(ctx context.Context) error {
	messages, err := r.store.ClaimDue(ctx, r.cfg.BatchSize, r.cfg.Lease)
	if err != nil {
		return fmt.Errorf("claim outbox: %w", err)
	}
	if len(messages) == 0 {
		return nil
	}

	published := make([]int64, 0, len(messages))
	for _, msg := range messages {
		if err := r.publisher.Publish(ctx, msg); err != nil {
			delay := Backoff(msg.Attempts, r.cfg.BaseBackoff, r.cfg.MaxBackoff)
			if resErr := r.store.Reschedule(ctx, msg.ID, time.Now().Add(delay), err.Error()); resErr != nil {
				return fmt.Errorf("reschedule outbox id=%d: %w", msg.ID, resErr)
			}
			slog.Error("outbox publish failed, backing off",
				slog.Int64("id", msg.ID),
				slog.Duration("backoff", delay),
				slog.Any("error", err),
			)
			continue
		}
		published = append(published, msg.ID)
	}

	if err := r.store.MarkPublished(ctx, published); err != nil {
		return fmt.Errorf("mark published: %w", err)
	}
	return nil
}
