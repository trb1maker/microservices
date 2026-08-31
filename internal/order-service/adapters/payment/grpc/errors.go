package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/trb1maker/microservices/internal/order-service/app"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func wrapRPCError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %w", app.ErrPaymentUnavailable, err)
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() { //nolint:exhaustive // only transport-related codes map to 503
		case codes.DeadlineExceeded, codes.Unavailable, codes.Canceled:
			return fmt.Errorf("%w: %w", app.ErrPaymentUnavailable, err)
		default:
			return err
		}
	}
	return err
}
