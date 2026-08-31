package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/internal/order-service/adapters/payment/grpc"
	"github.com/trb1maker/microservices/internal/order-service/app"
	paymentpb "github.com/trb1maker/microservices/internal/platform/proto/payment"
	grpcserver "google.golang.org/grpc"
)

type slowPaymentServer struct {
	paymentpb.UnimplementedPaymentServiceServer
	delay time.Duration
}

func (s *slowPaymentServer) Charge(_ context.Context, _ *paymentpb.ChargeRequest) (*paymentpb.ChargeResponse, error) {
	time.Sleep(s.delay)
	return &paymentpb.ChargeResponse{Status: paymentpb.PaymentStatus_SUCCEEDED}, nil
}

func TestPaymentClient_Charge_respectsRPCTimeout(t *testing.T) {
	t.Parallel()

	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	srv := grpcserver.NewServer()
	paymentpb.RegisterPaymentServiceServer(srv, &slowPaymentServer{delay: 500 * time.Millisecond})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GracefulStop)

	client, err := grpc.NewPaymentClient(context.Background(), grpc.ClientConfig{
		Addr:       lis.Addr().String(),
		Insecure:   true,
		RPCTimeout: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	start := time.Now()
	_, _, _, err = client.Charge(context.Background(), "order-1", "user-1", 1000)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.ErrorIs(t, err, app.ErrPaymentUnavailable)
	assert.Less(t, elapsed, 200*time.Millisecond)
}
