package app

import (
	"context"
	"time"
)

type ReceiptStorage interface {
	Exists(ctx context.Context, orderID string) (bool, error)
	Save(ctx context.Context, receipt Receipt) error
}

type SummaryRepository interface {
	RecordOrder(ctx context.Context, orderID string, amount int64, finalizedAt time.Time) (alreadyProcessed bool, err error)
}
