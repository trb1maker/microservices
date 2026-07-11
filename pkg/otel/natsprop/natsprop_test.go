package natsprop_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/trb1maker/microservices/pkg/otel/natsprop"
)

func TestInjectExtract_roundTrip(t *testing.T) {
	t.Parallel()

	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "publish")
	defer span.End()

	msg := &nats.Msg{Subject: "orders.created"}
	natsprop.Inject(ctx, msg)

	extracted := natsprop.Extract(context.Background(), msg)

	extractedSpan := trace.SpanFromContext(extracted)
	require.True(t, extractedSpan.SpanContext().IsValid())
	assert.Equal(t, span.SpanContext().TraceID(), extractedSpan.SpanContext().TraceID())
	assert.Equal(t, span.SpanContext().SpanID(), extractedSpan.SpanContext().SpanID())
}

func TestExtract_nilHeaderReturnsOriginalContext(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), testContextKey{}, "value")
	got := natsprop.Extract(ctx, &nats.Msg{Subject: "orders.created"})
	assert.Equal(t, "value", got.Value(testContextKey{}))
}

type testContextKey struct{}
