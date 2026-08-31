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
	"github.com/trb1maker/microservices/internal/platform/retry"
)

const (
	defaultMaxDeliver  = 10
	defaultNakDelay    = 2 * time.Second
	defaultMaxNakDelay = 30 * time.Second
	defaultNakJitter   = 0.2
)

var ErrStreamRequired = errors.New("jetstream stream is required")

// Handler processes a NATS message. Return non-nil error to trigger Nak/redelivery.
type Handler func(ctx context.Context, msg *nats.Msg) error

// DurableConsumerConfig configures JetStream durable consumer behavior.
type Inbox interface {
	Seen(ctx context.Context, eventID string) (bool, error)
	Mark(ctx context.Context, eventID string) error
}

type DurableConsumerConfig struct {
	DeliverPolicy jetstream.DeliverPolicy
	MaxDeliver    int
	NakDelay      time.Duration
	Inbox         Inbox
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
		handleDurableMessage(ctx, msg, handler, cfg, durable, nakDelay)
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

func handleDurableMessage(
	ctx context.Context,
	msg jetstream.Msg,
	handler Handler,
	cfg DurableConsumerConfig,
	durable string,
	nakDelay time.Duration,
) {
	nmsg := &nats.Msg{
		Subject: msg.Subject(),
		Data:    msg.Data(),
		Header:  msg.Headers(),
		Reply:   msg.Reply(),
	}
	handlerCtx := natsprop.Extract(ctx, nmsg)
	eventID := nmsg.Header.Get(HeaderMsgID)

	skip, ok := inboxAlreadySeen(handlerCtx, cfg.Inbox, eventID, durable, msg, nakDelay)
	if !ok {
		return
	}
	if skip {
		ackDurable(msg)
		return
	}

	if err := handler(handlerCtx, nmsg); err != nil {
		slog.Error("jetstream handler failed",
			slog.String("subject", nmsg.Subject),
			slog.String("durable", durable),
			slog.Any("error", err),
		)
		nakDurable(msg, nakDelay)
		return
	}

	if !markInbox(handlerCtx, cfg.Inbox, eventID, durable, msg, nakDelay) {
		return
	}
	ackDurable(msg)
}

func inboxAlreadySeen(
	ctx context.Context,
	box Inbox,
	eventID, durable string,
	msg jetstream.Msg,
	nakDelay time.Duration,
) (seen bool, ok bool) {
	if box == nil || eventID == "" {
		return false, true
	}
	seen, err := box.Seen(ctx, eventID)
	if err != nil {
		slog.Error("inbox lookup failed", slog.String("durable", durable), slog.Any("error", err))
		nakDurable(msg, nakDelay)
		return false, false
	}
	return seen, true
}

func markInbox(
	ctx context.Context,
	box Inbox,
	eventID, durable string,
	msg jetstream.Msg,
	nakDelay time.Duration,
) bool {
	if box == nil || eventID == "" {
		return true
	}
	if err := box.Mark(ctx, eventID); err != nil {
		slog.Error("inbox mark failed", slog.String("durable", durable), slog.Any("error", err))
		nakDurable(msg, nakDelay)
		return false
	}
	return true
}

func ackDurable(msg jetstream.Msg) {
	if err := msg.Ack(); err != nil {
		slog.Error("jetstream ack failed", slog.Any("error", err))
	}
}

func nakDurable(msg jetstream.Msg, baseDelay time.Duration) {
	delay := baseDelay
	if meta, err := msg.Metadata(); err == nil {
		attempt := int(meta.NumDelivered) //nolint:gosec // NumDelivered is bounded by MaxDeliver
		delay = retry.BackoffWithJitter(attempt, baseDelay, defaultMaxNakDelay, defaultNakJitter)
	}
	if err := msg.NakWithDelay(delay); err != nil {
		slog.Error("jetstream nak failed", slog.Any("error", err))
	}
}
