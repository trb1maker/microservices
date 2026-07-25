package notifier

import (
	"context"
	"log/slog"

	"github.com/trb1maker/microservices/internal/notification-service/app"
)

type SlogNotifier struct {
	serviceName string
}

func NewSlogNotifier(serviceName string) *SlogNotifier {
	return &SlogNotifier{serviceName: serviceName}
}

func (n *SlogNotifier) Notify(
	ctx context.Context,
	eventType, message, orderID, userID, transactionID string,
	amount int64,
) {
	attrs := []any{
		slog.String("service", n.serviceName),
		slog.String("event_type", eventType),
		slog.String("order_id", orderID),
		slog.String("user_id", userID),
	}
	if transactionID != "" {
		attrs = append(attrs, slog.String("transaction_id", transactionID))
	}
	if amount > 0 {
		attrs = append(attrs, slog.Int64("amount", amount))
	}
	slog.InfoContext(ctx, message, attrs...)
}

var _ app.NotificationSink = (*SlogNotifier)(nil)
