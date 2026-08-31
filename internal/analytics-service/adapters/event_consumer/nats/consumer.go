package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
	"github.com/trb1maker/microservices/internal/platform/natsx"
)

var deliverNew = natsx.DurableConsumerConfig{DeliverPolicy: jetstream.DeliverNewPolicy}

type Consumer struct {
	client  *natsx.Client
	subject string
	svc     *app.AnalyticsService
	sub     *natsx.Subscription
}

func NewConsumer(client *natsx.Client, subject string, svc *app.AnalyticsService) *Consumer {
	return &Consumer{client: client, subject: subject, svc: svc}
}

func (c *Consumer) Start(ctx context.Context) error {
	stream := natsx.StreamForSubject(c.subject)
	sub, err := c.client.ConsumeDurable(ctx, stream, "analytics-orders-finalized", c.subject, c.handleOrderFinalized, deliverNew)
	if err != nil {
		return fmt.Errorf("subscribe order finalized: %w", err)
	}
	c.sub = sub
	return nil
}

func (c *Consumer) Close() {
	if c.sub != nil {
		c.sub.Stop()
	}
}

func (c *Consumer) handleOrderFinalized(ctx context.Context, msg *nats.Msg) error {
	var event app.OrderFinalized
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal order finalized", slog.Any("error", err))
		return nil
	}
	if err := c.svc.ProcessOrderFinalized(ctx, event); err != nil {
		return fmt.Errorf("process order finalized: %w", err)
	}
	return nil
}
