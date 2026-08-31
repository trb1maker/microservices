package app

import (
	"context"
	"time"
)

type ReceiptStorage interface {
	Exists(ctx context.Context, orderID string) (bool, error)
	Save(ctx context.Context, receipt Receipt) error
	PresignGet(ctx context.Context, orderID string, expiry time.Duration) (url string, err error)
}

type SummaryRepository interface {
	IsOrderProcessed(ctx context.Context, orderID string) (bool, error)
	RecordOrder(ctx context.Context, orderID string, amount int64, finalizedAt time.Time) (alreadyProcessed bool, err error)
}

type ReceiptDocument struct {
	OrderID         string
	UserID          string
	TotalAmount     int64
	Status          string
	FinalizedAt     time.Time
	DeliveryAddress string
	SearchText      string
}

type SearchResult struct {
	OrderID         string    `json:"order_id"`
	UserID          string    `json:"user_id"`
	TotalAmount     int64     `json:"total_amount"`
	Status          string    `json:"status"`
	FinalizedAt     time.Time `json:"finalized_at"`
	DeliveryAddress string    `json:"delivery_address"`
}

type DocumentRepository interface {
	Upsert(ctx context.Context, doc ReceiptDocument) error
	GetByOrderID(ctx context.Context, orderID string) (*ReceiptDocument, error)
	Search(ctx context.Context, userID, query string, limit int) ([]SearchResult, error)
}
