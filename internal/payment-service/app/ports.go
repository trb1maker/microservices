package app

import (
	"context"

	"github.com/trb1maker/microservices/internal/payment-service/domain"
	"github.com/trb1maker/microservices/internal/platform/events"
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
	GetByOrderAndType(ctx context.Context, orderID domain.OrderID, txType domain.TransactionType) (*domain.Transaction, error)
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

type (
	PaymentSucceededEvent = events.PaymentSucceeded
	PaymentFailedEvent    = events.PaymentFailed
	RefundSucceededEvent  = events.RefundSucceeded
	RefundFailedEvent     = events.RefundFailed
)
