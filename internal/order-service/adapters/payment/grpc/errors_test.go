package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/internal/order-service/app"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWrapRPCError_mapsTransportToUnavailable(t *testing.T) {
	t.Parallel()

	require.NoError(t, wrapRPCError(nil))
	require.ErrorIs(t, wrapRPCError(context.DeadlineExceeded), app.ErrPaymentUnavailable)
	require.ErrorIs(t, wrapRPCError(context.Canceled), app.ErrPaymentUnavailable)
	require.ErrorIs(t, wrapRPCError(status.Error(codes.Unavailable, "down")), app.ErrPaymentUnavailable)
	require.ErrorIs(t, wrapRPCError(status.Error(codes.DeadlineExceeded, "slow")), app.ErrPaymentUnavailable)

	err := wrapRPCError(status.Error(codes.InvalidArgument, "bad request"))
	require.Error(t, err)
	require.NotErrorIs(t, err, app.ErrPaymentUnavailable)
}
