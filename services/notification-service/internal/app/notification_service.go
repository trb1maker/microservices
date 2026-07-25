package app

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrOrderFieldsRequired   = errors.New("order_id and user_id are required")
	ErrPaymentFieldsRequired = errors.New("order_id, user_id and transaction_id are required")
)

type NotificationService struct {
	sink NotificationSink
}

func NewNotificationService(sink NotificationSink) *NotificationService {
	return &NotificationService{sink: sink}
}

func (s *NotificationService) HandleOrderFinalized(ctx context.Context, event OrderFinalized) error {
	if event.OrderID == "" || event.UserID == "" {
		return ErrOrderFieldsRequired
	}
	message := fmt.Sprintf("[NOTIFICATION] Order %s confirmed. Thank you for your purchase!", event.OrderID)
	s.sink.Notify(ctx, "ORDER_FINALIZED", message, event.OrderID, event.UserID, "", 0)
	return nil
}

func (s *NotificationService) HandleOrderCancelled(ctx context.Context, event OrderCancelled) error {
	if event.OrderID == "" || event.UserID == "" {
		return ErrOrderFieldsRequired
	}
	message := fmt.Sprintf("[NOTIFICATION] Order %s has been cancelled.", event.OrderID)
	s.sink.Notify(ctx, "ORDER_CANCELLED", message, event.OrderID, event.UserID, "", 0)
	return nil
}

func (s *NotificationService) HandlePaymentSucceeded(ctx context.Context, event PaymentSucceeded) error {
	if event.OrderID == "" || event.UserID == "" || event.TransactionID == "" {
		return ErrPaymentFieldsRequired
	}
	message := fmt.Sprintf(
		"[NOTIFICATION] Payment %s for order %s succeeded. Amount: %d",
		event.TransactionID,
		event.OrderID,
		event.Amount,
	)
	s.sink.Notify(ctx, "PAYMENT_SUCCEEDED", message, event.OrderID, event.UserID, event.TransactionID, event.Amount)
	return nil
}

func (s *NotificationService) HandleRefundSucceeded(ctx context.Context, event RefundSucceeded) error {
	if event.OrderID == "" || event.UserID == "" || event.TransactionID == "" {
		return ErrPaymentFieldsRequired
	}
	message := fmt.Sprintf(
		"[NOTIFICATION] Refund %s for order %s processed. Amount: %d",
		event.TransactionID,
		event.OrderID,
		event.Amount,
	)
	s.sink.Notify(ctx, "REFUND_SUCCEEDED", message, event.OrderID, event.UserID, event.TransactionID, event.Amount)
	return nil
}
