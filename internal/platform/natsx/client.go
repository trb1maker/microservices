package natsx

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Client wraps a NATS connection and JetStream context.
type Client struct {
	conn *nats.Conn
	js   jetstream.JetStream
}

// New creates a JetStream client and ensures required streams exist.
func New(ctx context.Context, conn *nats.Conn) (*Client, error) {
	js, err := jetstream.New(conn)
	if err != nil {
		return nil, fmt.Errorf("create jetstream: %w", err)
	}

	c := &Client{conn: conn, js: js}
	if err := c.EnsureStreams(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

// Conn returns the underlying NATS connection.
func (c *Client) Conn() *nats.Conn {
	return c.conn
}

// JetStream returns the underlying JetStream interface.
func (c *Client) JetStream() jetstream.JetStream {
	return c.js
}

// EnsureStreams idempotently creates or updates JetStream streams.
func (c *Client) EnsureStreams(ctx context.Context) error {
	for _, cfg := range defaultStreams {
		if _, err := c.js.CreateOrUpdateStream(ctx, cfg); err != nil {
			return fmt.Errorf("ensure stream %s: %w", cfg.Name, err)
		}
	}
	return nil
}

// Publish publishes a payload to JetStream synchronously.
func (c *Client) Publish(ctx context.Context, subject string, payload []byte) error {
	_, err := c.js.Publish(ctx, subject, payload)
	if err != nil {
		return fmt.Errorf("jetstream publish %s: %w", subject, err)
	}
	return nil
}

// PublishMsg publishes a NATS message via JetStream synchronously.
func (c *Client) PublishMsg(ctx context.Context, msg *nats.Msg) error {
	_, err := c.js.PublishMsg(ctx, msg)
	if err != nil {
		return fmt.Errorf("jetstream publish msg %s: %w", msg.Subject, err)
	}
	return nil
}
