package natsx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/trb1maker/microservices/internal/platform/otel/natsprop"
)

const (
	defaultMaxDeliver = 10
	defaultNakDelay   = 2 * time.Second
)

var ErrStreamRequired = errors.New("jetstream stream is required")

// Handler processes a NATS message. Return non-nil error to trigger Nak/redelivery.
type Handler func(ctx context.Context, msg *nats.Msg) error

// DurableConsumerConfig configures JetStream durable consumer behavior.
type DurableConsumerConfig struct {
	DeliverPolicy jetstream.DeliverPolicy
	MaxDeliver    int
	NakDelay      time.Duration
}

// Subscription represents an active JetStream durable consumer.
type Subscription struct {
	consumeCtx jetstream.ConsumeContext
}

// Stop stops message consumption.
func (s *Subscription) Stop() {
	if s == nil || s.consumeCtx == nil {
		return
	}
	s.consumeCtx.Stop()
}

// ConsumeDurable starts a durable JetStream consumer with explicit ack.
func (c *Client) ConsumeDurable(
	ctx context.Context,
	stream string,
	durable string,
	filterSubject string,
	handler Handler,
	cfg DurableConsumerConfig,
) (*Subscription, error) {
	if stream == "" {
		return nil, fmt.Errorf("%w for subject %s", ErrStreamRequired, filterSubject)
	}

	maxDeliver := cfg.MaxDeliver
	if maxDeliver <= 0 {
		maxDeliver = defaultMaxDeliver
	}
	nakDelay := cfg.NakDelay
	if nakDelay <= 0 {
		nakDelay = defaultNakDelay
	}

	consumerCfg := jetstream.ConsumerConfig{
		Durable:       durable,
		FilterSubject: filterSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    maxDeliver,
	}
	if cfg.DeliverPolicy != 0 {
		consumerCfg.DeliverPolicy = cfg.DeliverPolicy
	}

	consumer, err := c.js.CreateOrUpdateConsumer(ctx, stream, consumerCfg)
	if err != nil {
		return nil, fmt.Errorf("create consumer %s on %s: %w", durable, stream, err)
	}

	consumeCtx, err := consumer.Consume(func(msg jetstream.Msg) {
		nmsg := &nats.Msg{
			Subject: msg.Subject(),
			Data:    msg.Data(),
			Header:  msg.Headers(),
			Reply:   msg.Reply(),
		}

		if err := handler(natsprop.Extract(ctx, nmsg), nmsg); err != nil {
			slog.Error("jetstream handler failed",
				slog.String("subject", nmsg.Subject),
				slog.String("durable", durable),
				slog.Any("error", err),
			)
			if nakErr := msg.NakWithDelay(nakDelay); nakErr != nil {
				slog.Error("jetstream nak failed", slog.Any("error", nakErr))
			}
			return
		}

		if ackErr := msg.Ack(); ackErr != nil {
			slog.Error("jetstream ack failed", slog.Any("error", ackErr))
		}
	})
	if err != nil {
		return nil, fmt.Errorf("consume %s: %w", durable, err)
	}

	slog.Info("jetstream durable consumer started",
		slog.String("stream", stream),
		slog.String("durable", durable),
		slog.String("subject", filterSubject),
	)

	return &Subscription{consumeCtx: consumeCtx}, nil
}
