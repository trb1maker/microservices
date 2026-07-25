package app

import (
	"context"

	"github.com/trb1maker/microservices/services/payment-service/internal/domain"
)

// AccountRepository defines the interface for account storage.
type AccountRepository interface {
	Get(ctx context.Context, userID domain.UserID) (*domain.Account, error)
	// UpdateBalance atomically updates the balance using optimistic locking.
	// Returns domain.ErrConcurrentModification if version mismatch.
	UpdateBalance(ctx context.Context, account *domain.Account) error
}

// TransactionRepository defines the interface for transaction storage.
type TransactionRepository interface {
	Create(ctx context.Context, tx *domain.Transaction) error
	Get(ctx context.Context, id domain.TransactionID) (*domain.Transaction, error)
	// GetRefundForOriginal returns a refund transaction for the given original charge transaction.
	GetRefundForOriginal(ctx context.Context, originalID domain.TransactionID) (*domain.Transaction, error)
	UpdateStatus(ctx context.Context, id domain.TransactionID, status domain.TransactionStatus, failureReason string) error
}

// EventPublisher defines the interface for publishing payment events.
type EventPublisher interface {
	PublishPaymentSucceeded(ctx context.Context, event PaymentSucceededEvent) error
	PublishPaymentFailed(ctx context.Context, event PaymentFailedEvent) error
	PublishRefundSucceeded(ctx context.Context, event RefundSucceededEvent) error
	PublishRefundFailed(ctx context.Context, event RefundFailedEvent) error
}

// PaymentSucceededEvent is published after a successful charge.
type PaymentSucceededEvent struct {
	OrderID       string `json:"order_id"`
	UserID        string `json:"user_id"`
	Amount        int64  `json:"amount"`
	TransactionID string `json:"transaction_id"`
	Timestamp     string `json:"timestamp"`
}

// PaymentFailedEvent is published after a failed charge.
type PaymentFailedEvent struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	Amount    int64  `json:"amount"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// RefundSucceededEvent is published after a successful refund.
type RefundSucceededEvent struct {
	OrderID               string `json:"order_id"`
	UserID                string `json:"user_id"`
	Amount                int64  `json:"amount"`
	TransactionID         string `json:"transaction_id"`
	OriginalTransactionID string `json:"original_transaction_id"`
	Timestamp             string `json:"timestamp"`
}

// RefundFailedEvent is published after a failed refund.
type RefundFailedEvent struct {
	OrderID               string `json:"order_id"`
	UserID                string `json:"user_id"`
	Amount                int64  `json:"amount"`
	OriginalTransactionID string `json:"original_transaction_id"`
	Reason                string `json:"reason"`
	Timestamp             string `json:"timestamp"`
}
