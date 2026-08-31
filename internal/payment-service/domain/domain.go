package domain

import "uuid"

// UserID is a domain identifier for a user.
type UserID string

// OrderID is a domain identifier for an order.
type OrderID string

// TransactionID is a domain identifier for a payment transaction.
type TransactionID string

// TransactionType represents the type of payment transaction.
type TransactionType string

const (
	TransactionTypeCharge TransactionType = "charge"
	TransactionTypeRefund TransactionType = "refund"
)

// TransactionStatus represents the status of a payment transaction.
type TransactionStatus string

const (
	TransactionStatusPending   TransactionStatus = "PENDING"
	TransactionStatusSucceeded TransactionStatus = "SUCCEEDED"
	TransactionStatusFailed    TransactionStatus = "FAILED"
)

// Account represents a user's payment account with balance.
type Account struct {
	UserID  UserID
	Balance int64 // in minor units (cents)
	Version int   // optimistic lock version
}

// Transaction represents a single payment operation.
type Transaction struct {
	ID                    TransactionID
	OrderID               OrderID
	UserID                UserID
	Type                  TransactionType
	Amount                int64
	Status                TransactionStatus
	OriginalTransactionID *TransactionID // set for refunds
	FailureReason         string
}

// NewTransactionID generates a new unique transaction ID.
func NewTransactionID() TransactionID {
	return TransactionID(uuid.NewV7().String())
}
