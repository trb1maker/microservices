package natstest

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/trb1maker/microservices/internal/platform/natsx"
)

const (
	Image = "nats:2.14-alpine"

	connectTimeout    = 2 * time.Second
	connectRetryFor   = 20 * time.Second
	connectRetryEvery = 200 * time.Millisecond
	monitoringPort    = "8222/tcp"
)

// ContainerOptions ждёт не только bind порта 4222, но и HTTP /healthz:
// иначе nats.Connect ловит EOF, пока сервер ещё поднимает JetStream.
func ContainerOptions() []testcontainers.ContainerCustomizer {
	return []testcontainers.ContainerCustomizer{
		testcontainers.WithCmd("-DV", "-js", "-m", "8222"),
		testcontainers.WithAdditionalWaitStrategy(
			wait.ForHTTP("/healthz").WithPort(monitoringPort),
		),
	}
}

// Connect повторяет попытку: Docker уже пробросил порт, а NATS ещё закрывает первый handshake.
func Connect(t *testing.T, url string, opts ...nats.Option) *nats.Conn {
	t.Helper()

	opts = append([]nats.Option{nats.Timeout(connectTimeout)}, opts...)

	var (
		conn *nats.Conn
		err  error
	)
	deadline := time.Now().Add(connectRetryFor)
	for {
		conn, err = nats.Connect(url, opts...)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			require.NoError(t, err, "nats connect %s", url)
		}
		time.Sleep(connectRetryEvery)
	}
}

// NewClient connects to NATS and initializes JetStream streams.
func NewClient(t *testing.T, url string, opts ...nats.Option) *natsx.Client {
	t.Helper()

	conn := Connect(t, url, opts...)
	client, err := natsx.New(context.Background(), conn)
	require.NoError(t, err)
	return client
}
