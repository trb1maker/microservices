package natsx_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/trb1maker/microservices/internal/platform/natsx"
	"github.com/trb1maker/microservices/internal/platform/otel/natsprop"
)

const natsTestImage = "nats:2.14-alpine"

func natsContainerOptions() []testcontainers.ContainerCustomizer {
	return []testcontainers.ContainerCustomizer{
		testcontainers.WithCmd("-DV", "-js", "-m", "8222"),
		testcontainers.WithAdditionalWaitStrategy(
			wait.ForHTTP("/healthz").WithPort("8222/tcp"),
		),
	}
}

func connectNATS(t *testing.T, url string) *nats.Conn {
	t.Helper()

	opts := []nats.Option{nats.Timeout(2 * time.Second)}
	deadline := time.Now().Add(20 * time.Second)
	for {
		conn, err := nats.Connect(url, opts...)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			require.NoError(t, err, "nats connect %s", url)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestStreamForSubject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		subject string
		stream  string
	}{
		{"orders.created", "ORDERS"},
		{"orders.finalized", "ORDERS"},
		{"cart.reserve_items", "CART"},
		{"store.items_reserved", "STORE"},
		{"payment.succeeded", "PAYMENT"},
		{"unknown.topic", ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.stream, natsx.StreamForSubject(tt.subject))
	}
}

func TestPublishAndConsumeDurable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	container, err := tcnats.Run(ctx, natsTestImage, natsContainerOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	url, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	nc := connectNATS(t, url)
	t.Cleanup(nc.Close)

	client, err := natsx.New(ctx, nc)
	require.NoError(t, err)

	const subject = "orders.created"
	received := make(chan *nats.Msg, 1)
	sub, err := client.ConsumeDurable(ctx, "ORDERS", "test-orders-created", subject, func(_ context.Context, msg *nats.Msg) error {
		received <- msg
		return nil
	}, natsx.DurableConsumerConfig{})
	require.NoError(t, err)
	t.Cleanup(sub.Stop)

	payload := []byte(`{"order_id":"test"}`)
	require.NoError(t, client.Publish(ctx, subject, payload))

	select {
	case msg := <-received:
		require.Equal(t, payload, msg.Data)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestPublishAndConsumeDurable_propagatesTrace(t *testing.T) {
	ctx := context.Background()
	container, err := tcnats.Run(ctx, natsTestImage, natsContainerOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	url, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	nc := connectNATS(t, url)
	t.Cleanup(nc.Close)

	client, err := natsx.New(ctx, nc)
	require.NoError(t, err)

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	tracer := tp.Tracer("natsx-test")
	publishCtx, span := tracer.Start(ctx, "publish")
	parent := span.SpanContext()
	span.End()

	const subject = "orders.created"
	got := make(chan trace.SpanContext, 1)
	sub, err := client.ConsumeDurable(ctx, "ORDERS", "test-trace-propagate", subject, func(msgCtx context.Context, _ *nats.Msg) error {
		got <- trace.SpanFromContext(msgCtx).SpanContext()
		return nil
	}, natsx.DurableConsumerConfig{})
	require.NoError(t, err)
	t.Cleanup(sub.Stop)

	msg := &nats.Msg{Subject: subject, Data: []byte(`{"ok":true}`)}
	natsprop.Inject(publishCtx, msg)
	require.NoError(t, client.PublishMsg(publishCtx, msg))

	select {
	case sc := <-got:
		require.True(t, sc.IsValid())
		require.Equal(t, parent.TraceID(), sc.TraceID())
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for traced message")
	}
}
