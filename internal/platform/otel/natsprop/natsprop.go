package natsprop

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// Inject writes the active trace context into NATS message headers.
func Inject(ctx context.Context, msg *nats.Msg) {
	if msg.Header == nil {
		msg.Header = make(nats.Header)
	}

	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(msg.Header))
}

// Extract restores trace context from NATS message headers.
func Extract(ctx context.Context, msg *nats.Msg) context.Context {
	if msg == nil || msg.Header == nil {
		return ctx
	}

	return otel.GetTextMapPropagator().Extract(ctx, headerCarrier(msg.Header))
}

type headerCarrier nats.Header

func (c headerCarrier) Get(key string) string {
	return nats.Header(c).Get(key)
}

func (c headerCarrier) Set(key, value string) {
	nats.Header(c).Set(key, value)
}

func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}

	return keys
}

var _ propagation.TextMapCarrier = headerCarrier{}
