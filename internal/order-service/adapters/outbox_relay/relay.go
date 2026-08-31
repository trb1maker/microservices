package relay

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/order-service/app"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	"github.com/trb1maker/microservices/internal/platform/otel/natsprop"
)

type OutboxStore interface {
	ProcessUnpublishedBatch(
		ctx context.Context,
		limit int,
		publish func(messages []app.OutboxMessage) ([]int64, error),
	) error
}

type Config struct {
	PollInterval time.Duration
	BatchSize    int
}

const (
	defaultPollInterval = 500 * time.Millisecond
	defaultBatchSize    = 50
)

type Relay struct {
	store  OutboxStore
	client *natsx.Client
	cfg    Config
}

func New(store OutboxStore, client *natsx.Client, cfg Config) *Relay {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	return &Relay{store: store, client: client, cfg: cfg}
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
	var publishedCount int
	err := r.store.ProcessUnpublishedBatch(ctx, r.cfg.BatchSize, func(messages []app.OutboxMessage) ([]int64, error) {
		publishedIDs := make([]int64, 0, len(messages))
		for _, msg := range messages {
			nmsg := &nats.Msg{Subject: msg.Subject, Data: msg.Payload}
			if msg.AggregateID != uuid.Nil {
				if nmsg.Header == nil {
					nmsg.Header = nats.Header{}
				}
				nmsg.Header.Set("X-Order-ID", msg.AggregateID.String())
			}
			natsprop.Inject(ctx, nmsg)

			if err := r.client.PublishMsg(ctx, nmsg); err != nil {
				return nil, fmt.Errorf("publish outbox id=%d: %w", msg.ID, err)
			}
			publishedIDs = append(publishedIDs, msg.ID)
		}
		publishedCount = len(publishedIDs)
		return publishedIDs, nil
	})
	if err != nil {
		return fmt.Errorf("process outbox batch: %w", err)
	}

	if publishedCount > 0 {
		slog.Debug("outbox relay published batch", slog.Int("count", publishedCount))
	}
	return nil
}
