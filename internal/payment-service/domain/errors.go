package domain

import "errors"

var (
	ErrInsufficientFunds          = errors.New("insufficient funds")
	ErrAccountNotFound            = errors.New("account not found")
	ErrTransactionNotFound        = errors.New("transaction not found")
	ErrInvalidAmount              = errors.New("amount must be positive")
	ErrAlreadyRefunded            = errors.New("transaction has already been refunded")
	ErrInvalidOriginalTransaction = errors.New("original transaction not found or not succeeded")
	ErrConcurrentModification     = errors.New("concurrent modification detected")
	ErrDuplicateTransaction       = errors.New("transaction already exists")
)
