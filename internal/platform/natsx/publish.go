package natsx

import (
	"context"
	"encoding/json"
	"fmt"
	"uuid"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/platform/otel/natsprop"
)

const (
	HeaderOrderID = "X-Order-ID"
	HeaderMsgID   = "Nats-Msg-Id"
)

func (c *Client) PublishJSON(ctx context.Context, subject string, payload any, headers nats.Header) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	msg := &nats.Msg{Subject: subject, Data: data, Header: headers}
	if msg.Header == nil {
		msg.Header = nats.Header{}
	}
	if msg.Header.Get(HeaderMsgID) == "" {
		msg.Header.Set(HeaderMsgID, uuid.NewV7().String())
	}
	natsprop.Inject(ctx, msg)

	if err := c.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}
