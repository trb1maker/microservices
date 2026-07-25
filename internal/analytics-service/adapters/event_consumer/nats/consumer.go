package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
)

type Consumer struct {
	conn    *nats.Conn
	subject string
	svc     *app.AnalyticsService
	sub     *nats.Subscription
}

func NewConsumer(conn *nats.Conn, subject string, svc *app.AnalyticsService) *Consumer {
	return &Consumer{conn: conn, subject: subject, svc: svc}
}

func (c *Consumer) Start() error {
	sub, err := c.conn.Subscribe(c.subject, c.handleOrderFinalized)
	if err != nil {
		return fmt.Errorf("subscribe order finalized: %w", err)
	}
	c.sub = sub
	return nil
}

func (c *Consumer) Close() {
	if c.sub != nil {
		_ = c.sub.Unsubscribe()
	}
}

func (c *Consumer) handleOrderFinalized(msg *nats.Msg) {
	var event app.OrderFinalized
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		slog.Error("unmarshal order finalized", slog.Any("error", err))
		return
	}
	if err := c.svc.ProcessOrderFinalized(context.Background(), event); err != nil {
		slog.Error("process order finalized", slog.Any("error", err))
	}
}
