package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trb1maker/microservices/services/order-service/internal/app"

	"github.com/nats-io/nats.go"
)

type Subjects struct {
	OrderCreated       string
	ReserveItems       string
	ConfirmOrder       string
	ReleaseReservation string
	OrderFinalized     string
	OrderCancelled     string
}

type Publisher struct {
	conn     *nats.Conn
	subjects Subjects
}

func NewPublisher(conn *nats.Conn, subjects Subjects) *Publisher {
	return &Publisher{conn: conn, subjects: subjects}
}

func (p *Publisher) PublishReserveItems(ctx context.Context, event app.ReserveItems) error {
	return p.publishJSON(ctx, p.subjects.ReserveItems, event)
}

func (p *Publisher) PublishReleaseReservation(ctx context.Context, event app.ReleaseReservation) error {
	return p.publishJSON(ctx, p.subjects.ReleaseReservation, event)
}

func (p *Publisher) PublishOrderCreated(ctx context.Context, event app.OrderCreated) error {
	return p.publishJSON(ctx, p.subjects.OrderCreated, event)
}

func (p *Publisher) PublishConfirmOrder(ctx context.Context, event app.ConfirmOrder) error {
	return p.publishJSON(ctx, p.subjects.ConfirmOrder, event)
}

func (p *Publisher) PublishOrderFinalized(ctx context.Context, event app.OrderFinalized) error {
	return p.publishJSON(ctx, p.subjects.OrderFinalized, event)
}

func (p *Publisher) PublishOrderCancelled(ctx context.Context, event app.OrderCancelled) error {
	return p.publishJSON(ctx, p.subjects.OrderCancelled, event)
}

func (p *Publisher) IsConnected() bool {
	return p.conn != nil && p.conn.IsConnected()
}

func (p *Publisher) publishJSON(_ context.Context, subject string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if err := p.conn.Publish(subject, payload); err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	return nil
}
