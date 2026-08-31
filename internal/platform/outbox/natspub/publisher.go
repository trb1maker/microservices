package natspub

import (
	"context"
	"fmt"
	"uuid"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	"github.com/trb1maker/microservices/internal/platform/otel/natsprop"
	"github.com/trb1maker/microservices/internal/platform/outbox"
)

type Publisher struct {
	client *natsx.Client
}

func New(client *natsx.Client) *Publisher {
	return &Publisher{client: client}
}

func (p *Publisher) Publish(ctx context.Context, msg outbox.Message) error {
	nmsg := &nats.Msg{
		Subject: msg.Subject,
		Data:    msg.Payload,
		Header:  nats.Header{},
	}
	nmsg.Header.Set(natsx.HeaderMsgID, fmt.Sprintf("outbox-%d", msg.ID))
	if msg.AggregateID != uuid.Nil() {
		nmsg.Header.Set(natsx.HeaderOrderID, msg.AggregateID.String())
	}
	natsprop.Inject(ctx, nmsg)
	if err := p.client.PublishMsg(ctx, nmsg); err != nil {
		return fmt.Errorf("publish outbox id=%d: %w", msg.ID, err)
	}
	return nil
}

var _ outbox.Publisher = (*Publisher)(nil)
