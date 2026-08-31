package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trb1maker/microservices/internal/order-service/app"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/platform/natsx"
	"github.com/trb1maker/microservices/internal/platform/otel/natsprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const headerOrderID = "X-Order-ID"

var tracer = otel.Tracer("order-service/nats")

type Subjects struct {
	OrderCreated       string
	ReserveItems       string
	ConfirmOrder       string
	ReleaseReservation string
	OrderFinalized     string
	OrderCancelled     string
}

type Publisher struct {
	client   *natsx.Client
	subjects Subjects
}

func NewPublisher(client *natsx.Client, subjects Subjects) *Publisher {
	return &Publisher{client: client, subjects: subjects}
}

func (p *Publisher) PublishReserveItems(ctx context.Context, event app.ReserveItems, orderID string) error {
	return p.publishJSON(ctx, p.subjects.ReserveItems, event, orderID)
}

func (p *Publisher) PublishReleaseReservation(ctx context.Context, event app.ReleaseReservation) error {
	return p.publishJSON(ctx, p.subjects.ReleaseReservation, event, event.OrderID)
}

func (p *Publisher) PublishOrderCreated(ctx context.Context, event app.OrderCreated) error {
	return p.publishJSON(ctx, p.subjects.OrderCreated, event, event.OrderID)
}

func (p *Publisher) PublishConfirmOrder(ctx context.Context, event app.ConfirmOrder) error {
	return p.publishJSON(ctx, p.subjects.ConfirmOrder, event, event.OrderID)
}

func (p *Publisher) PublishOrderFinalized(ctx context.Context, event app.OrderFinalized) error {
	return p.publishJSON(ctx, p.subjects.OrderFinalized, event, event.OrderID)
}

func (p *Publisher) PublishOrderCancelled(ctx context.Context, event app.OrderCancelled) error {
	return p.publishJSON(ctx, p.subjects.OrderCancelled, event, event.OrderID)
}

func (p *Publisher) IsConnected() bool {
	return p.client != nil && p.client.Conn().IsConnected()
}

func (p *Publisher) publishJSON(ctx context.Context, subject string, event any, orderID string) error {
	_, span := tracer.Start(ctx, "nats.publish")
	defer span.End()

	span.SetAttributes(
		semconv.MessagingSystemKey.String("nats"),
		attribute.String("messaging.destination", subject),
	)

	payload, err := json.Marshal(event)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return fmt.Errorf("marshal event: %w", err)
	}

	msg := &nats.Msg{Subject: subject, Data: payload}
	if orderID != "" {
		msg.Header = nats.Header{}
		msg.Header.Set(headerOrderID, orderID)
	}
	natsprop.Inject(ctx, msg)

	if err := p.client.PublishMsg(ctx, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return fmt.Errorf("publish message: %w", err)
	}

	return nil
}
