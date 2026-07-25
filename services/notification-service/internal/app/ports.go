package app

import "context"

type NotificationSink interface {
	Notify(ctx context.Context, eventType, message, orderID, userID, transactionID string, amount int64)
}
