package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/trb1maker/microservices/internal/payment-service/domain"
)

// PaymentService implements the payment business logic.
type PaymentService struct {
	accounts     AccountRepository
	transactions TransactionRepository
	events       EventPublisher
}

// NewPaymentService creates a new PaymentService.
func NewPaymentService(
	accounts AccountRepository,
	transactions TransactionRepository,
	events EventPublisher,
) *PaymentService {
	return &PaymentService{
		accounts:     accounts,
		transactions: transactions,
		events:       events,
	}
}

// ChargeResult holds the result of a charge operation.
type ChargeResult struct {
	TransactionID string
	Status        domain.TransactionStatus
	Message       string
}

// Charge processes a payment for an order.
func (s *PaymentService) Charge(ctx context.Context, orderID, userID string, amount int64) (*ChargeResult, error) {
	if amount <= 0 {
		return &ChargeResult{
			Status:  domain.TransactionStatusFailed,
			Message: domain.ErrInvalidAmount.Error(),
		}, nil
	}

	account, err := s.accounts.Get(ctx, domain.UserID(userID))
	if err != nil {
		return s.handleChargeAccountError(ctx, orderID, userID, amount, err)
	}

	if account.Balance < amount {
		return s.handleChargeInsufficientFunds(ctx, orderID, userID, amount)
	}

	return s.processCharge(ctx, orderID, userID, amount)
}

func (s *PaymentService) handleChargeAccountError(ctx context.Context, orderID, userID string, amount int64, err error) (*ChargeResult, error) {
	if errors.Is(err, domain.ErrAccountNotFound) {
		return &ChargeResult{
			Status:  domain.TransactionStatusFailed,
			Message: "account not found",
		}, nil
	}
	return nil, fmt.Errorf("get account: %w", err)
}

func (s *PaymentService) handleChargeInsufficientFunds(ctx context.Context, orderID, userID string, amount int64) (*ChargeResult, error) {
	tx := &domain.Transaction{
		ID:            domain.NewTransactionID(),
		OrderID:       domain.OrderID(orderID),
		UserID:        domain.UserID(userID),
		Type:          domain.TransactionTypeCharge,
		Amount:        amount,
		Status:        domain.TransactionStatusFailed,
		FailureReason: domain.ErrInsufficientFunds.Error(),
	}
	if err := s.transactions.Create(ctx, tx); err != nil {
		return nil, fmt.Errorf("create failed transaction: %w", err)
	}

	s.publishPaymentFailed(ctx, orderID, userID, amount, "insufficient funds")

	return &ChargeResult{
		TransactionID: string(tx.ID),
		Status:        domain.TransactionStatusFailed,
		Message:       "insufficient funds",
	}, nil
}

func (s *PaymentService) processCharge(ctx context.Context, orderID, userID string, amount int64) (*ChargeResult, error) {
	tx := &domain.Transaction{
		ID:      domain.NewTransactionID(),
		OrderID: domain.OrderID(orderID),
		UserID:  domain.UserID(userID),
		Type:    domain.TransactionTypeCharge,
		Amount:  amount,
		Status:  domain.TransactionStatusPending,
	}
	if err := s.transactions.Create(ctx, tx); err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}

	const maxRetries = 10
	for range maxRetries {
		account, err := s.accounts.Get(ctx, domain.UserID(userID))
		if err != nil {
			return nil, fmt.Errorf("get account: %w", err)
		}
		if account.Balance < amount {
			if err := s.transactions.UpdateStatus(ctx, tx.ID, domain.TransactionStatusFailed, domain.ErrInsufficientFunds.Error()); err != nil {
				return nil, fmt.Errorf("update failed transaction status: %w", err)
			}
			s.publishPaymentFailed(ctx, orderID, userID, amount, "insufficient funds")
			return &ChargeResult{
				TransactionID: string(tx.ID),
				Status:        domain.TransactionStatusFailed,
				Message:       "insufficient funds",
			}, nil
		}

		account.Balance -= amount
		if err := s.accounts.UpdateBalance(ctx, account); err != nil {
			if errors.Is(err, domain.ErrConcurrentModification) {
				continue
			}
			return nil, fmt.Errorf("update balance: %w", err)
		}

		if err := s.transactions.UpdateStatus(ctx, tx.ID, domain.TransactionStatusSucceeded, ""); err != nil {
			return nil, fmt.Errorf("update transaction status: %w", err)
		}

		s.publishPaymentSucceeded(ctx, orderID, userID, amount, string(tx.ID))

		slog.Info("charge succeeded",
			slog.String("transaction_id", string(tx.ID)),
			slog.String("order_id", orderID),
			slog.String("user_id", userID),
			slog.Int64("amount", amount),
		)

		return &ChargeResult{
			TransactionID: string(tx.ID),
			Status:        domain.TransactionStatusSucceeded,
			Message:       "payment succeeded",
		}, nil
	}

	return s.handleChargeConcurrentModification(ctx, tx, domain.ErrConcurrentModification)
}

func (s *PaymentService) handleChargeConcurrentModification(ctx context.Context, tx *domain.Transaction, err error) (*ChargeResult, error) {
	if errors.Is(err, domain.ErrConcurrentModification) {
		_ = s.transactions.UpdateStatus(ctx, tx.ID, domain.TransactionStatusFailed, "concurrent modification")
		return &ChargeResult{
			TransactionID: string(tx.ID),
			Status:        domain.TransactionStatusFailed,
			Message:       "concurrent modification, please retry",
		}, nil
	}
	return nil, fmt.Errorf("update balance: %w", err)
}

// RefundResult holds the result of a refund operation.
type RefundResult struct {
	TransactionID string
	Status        domain.TransactionStatus
	Message       string
}

// Refund processes a refund for a previously charged order.
func (s *PaymentService) Refund(ctx context.Context, orderID, userID string, amount int64, originalTransactionID string) (*RefundResult, error) {
	if amount <= 0 {
		return &RefundResult{
			Status:  domain.TransactionStatusFailed,
			Message: domain.ErrInvalidAmount.Error(),
		}, nil
	}

	origTx, err := s.transactions.Get(ctx, domain.TransactionID(originalTransactionID))
	if err != nil {
		return s.handleRefundOriginalNotFound(ctx, err)
	}

	if origTx.Status != domain.TransactionStatusSucceeded {
		return &RefundResult{
			Status:  domain.TransactionStatusFailed,
			Message: "original transaction not in succeeded state",
		}, nil
	}

	if existingRefund, err := s.checkExistingRefund(ctx, origTx.ID); err != nil {
		return nil, err
	} else if existingRefund != nil {
		return existingRefund, nil
	}

	return s.processRefund(ctx, orderID, userID, amount, origTx)
}

func (s *PaymentService) handleRefundOriginalNotFound(ctx context.Context, err error) (*RefundResult, error) {
	if errors.Is(err, domain.ErrTransactionNotFound) {
		return &RefundResult{
			Status:  domain.TransactionStatusFailed,
			Message: "original transaction not found",
		}, nil
	}
	return nil, fmt.Errorf("get original transaction: %w", err)
}

func (s *PaymentService) checkExistingRefund(ctx context.Context, origTxID domain.TransactionID) (*RefundResult, error) {
	existingRefund, err := s.transactions.GetRefundForOriginal(ctx, origTxID)
	if err != nil && !errors.Is(err, domain.ErrTransactionNotFound) {
		return nil, fmt.Errorf("check existing refund: %w", err)
	}
	if existingRefund != nil && existingRefund.Status == domain.TransactionStatusSucceeded {
		return &RefundResult{
			TransactionID: string(existingRefund.ID),
			Status:        domain.TransactionStatusFailed,
			Message:       "transaction has already been refunded",
		}, nil
	}
	return nil, nil
}

func (s *PaymentService) processRefund(ctx context.Context, orderID, userID string, amount int64, origTx *domain.Transaction) (*RefundResult, error) {
	origTxID := origTx.ID
	origTxIDStr := string(origTxID)
	refundTx := &domain.Transaction{
		ID:                    domain.NewTransactionID(),
		OrderID:               domain.OrderID(orderID),
		UserID:                domain.UserID(userID),
		Type:                  domain.TransactionTypeRefund,
		Amount:                amount,
		Status:                domain.TransactionStatusPending,
		OriginalTransactionID: &origTxID,
	}
	if err := s.transactions.Create(ctx, refundTx); err != nil {
		return nil, fmt.Errorf("create refund transaction: %w", err)
	}

	account, err := s.accounts.Get(ctx, domain.UserID(userID))
	if err != nil {
		return nil, fmt.Errorf("get account for refund: %w", err)
	}

	account.Balance += amount
	if err := s.accounts.UpdateBalance(ctx, account); err != nil {
		return s.handleRefundConcurrentModification(ctx, refundTx, err)
	}

	if err := s.transactions.UpdateStatus(ctx, refundTx.ID, domain.TransactionStatusSucceeded, ""); err != nil {
		return nil, fmt.Errorf("update refund status: %w", err)
	}

	s.publishRefundSucceeded(ctx, orderID, userID, amount, string(refundTx.ID), origTxIDStr)

	slog.Info("refund succeeded",
		slog.String("transaction_id", string(refundTx.ID)),
		slog.String("order_id", orderID),
		slog.String("user_id", userID),
		slog.Int64("amount", amount),
		slog.String("original_transaction_id", origTxIDStr),
	)

	return &RefundResult{
		TransactionID: string(refundTx.ID),
		Status:        domain.TransactionStatusSucceeded,
		Message:       "refund succeeded",
	}, nil
}

func (s *PaymentService) handleRefundConcurrentModification(ctx context.Context, refundTx *domain.Transaction, err error) (*RefundResult, error) {
	if errors.Is(err, domain.ErrConcurrentModification) {
		_ = s.transactions.UpdateStatus(ctx, refundTx.ID, domain.TransactionStatusFailed, "concurrent modification")
		return &RefundResult{
			TransactionID: string(refundTx.ID),
			Status:        domain.TransactionStatusFailed,
			Message:       "concurrent modification, please retry",
		}, nil
	}
	return nil, fmt.Errorf("update balance for refund: %w", err)
}

func (s *PaymentService) publishPaymentSucceeded(ctx context.Context, orderID, userID string, amount int64, transactionID string) {
	event := PaymentSucceededEvent{
		OrderID:       orderID,
		UserID:        userID,
		Amount:        amount,
		TransactionID: transactionID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.events.PublishPaymentSucceeded(ctx, event); err != nil {
		slog.Error("failed to publish payment succeeded event", slog.Any("error", err))
	}
}

func (s *PaymentService) publishPaymentFailed(ctx context.Context, orderID, userID string, amount int64, reason string) {
	event := PaymentFailedEvent{
		OrderID:   orderID,
		UserID:    userID,
		Amount:    amount,
		Reason:    reason,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.events.PublishPaymentFailed(ctx, event); err != nil {
		slog.Error("failed to publish payment failed event", slog.Any("error", err))
	}
}

func (s *PaymentService) publishRefundSucceeded(ctx context.Context, orderID, userID string, amount int64, transactionID, originalTransactionID string) {
	event := RefundSucceededEvent{
		OrderID:               orderID,
		UserID:                userID,
		Amount:                amount,
		TransactionID:         transactionID,
		OriginalTransactionID: originalTransactionID,
		Timestamp:             time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.events.PublishRefundSucceeded(ctx, event); err != nil {
		slog.Error("failed to publish refund succeeded event", slog.Any("error", err))
	}
}
