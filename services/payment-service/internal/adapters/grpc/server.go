package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/trb1maker/microservices/services/payment-service/internal/app"
	"github.com/trb1maker/microservices/services/payment-service/internal/domain"

	paymentpb "github.com/trb1maker/microservices/pkg/proto/payment"
)

// PaymentServer implements the payment.PaymentService gRPC server.
type PaymentServer struct {
	paymentpb.UnimplementedPaymentServiceServer
	svc *app.PaymentService
}

// NewPaymentServer creates a new PaymentServer.
func NewPaymentServer(svc *app.PaymentService) *PaymentServer {
	return &PaymentServer{svc: svc}
}

// Charge processes a payment for an order.
func (s *PaymentServer) Charge(ctx context.Context, req *paymentpb.ChargeRequest) (*paymentpb.ChargeResponse, error) {
	if req.GetOrderId() == "" || req.GetUserId() == "" {
		//nolint:wrapcheck // gRPC status must not be wrapped
		return nil, status.Error(codes.InvalidArgument, "order_id and user_id are required")
	}
	if req.GetAmount() <= 0 {
		//nolint:wrapcheck // gRPC status must not be wrapped
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	result, err := s.svc.Charge(ctx, req.GetOrderId(), req.GetUserId(), req.GetAmount())
	if err != nil {
		//nolint:wrapcheck // gRPC status must not be wrapped
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &paymentpb.ChargeResponse{
		TransactionId: result.TransactionID,
		Status:        toProtoStatus(result.Status),
		Message:       result.Message,
	}, nil
}

// Refund processes a refund for a previously charged order.
func (s *PaymentServer) Refund(ctx context.Context, req *paymentpb.RefundRequest) (*paymentpb.RefundResponse, error) {
	if req.GetOrderId() == "" || req.GetUserId() == "" || req.GetOriginalTransactionId() == "" {
		//nolint:wrapcheck // gRPC status must not be wrapped
		return nil, status.Error(codes.InvalidArgument, "order_id, user_id, and original_transaction_id are required")
	}
	if req.GetAmount() <= 0 {
		//nolint:wrapcheck // gRPC status must not be wrapped
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	result, err := s.svc.Refund(ctx, req.GetOrderId(), req.GetUserId(), req.GetAmount(), req.GetOriginalTransactionId())
	if err != nil {
		//nolint:wrapcheck // gRPC status must not be wrapped
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &paymentpb.RefundResponse{
		TransactionId: result.TransactionID,
		Status:        toProtoStatus(result.Status),
		Message:       result.Message,
	}, nil
}

func toProtoStatus(s domain.TransactionStatus) paymentpb.PaymentStatus {
	switch s {
	case domain.TransactionStatusSucceeded:
		return paymentpb.PaymentStatus_SUCCEEDED
	case domain.TransactionStatusFailed:
		return paymentpb.PaymentStatus_FAILED
	case domain.TransactionStatusPending:
		return paymentpb.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	default:
		return paymentpb.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	}
}

// Ensure compile-time interface compliance.
var _ paymentpb.PaymentServiceServer = (*PaymentServer)(nil)
