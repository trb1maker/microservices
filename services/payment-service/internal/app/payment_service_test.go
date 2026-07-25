package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/services/payment-service/internal/domain"
)

// mockAccountRepo implements AccountRepository for testing.
type mockAccountRepo struct {
	accounts map[string]*domain.Account
}

func newMockAccountRepo() *mockAccountRepo {
	return &mockAccountRepo{
		accounts: map[string]*domain.Account{
			"user-1": {UserID: "user-1", Balance: 100000, Version: 1},
			"user-2": {UserID: "user-2", Balance: 50000, Version: 1},
			"user-3": {UserID: "user-3", Balance: 0, Version: 1},
		},
	}
}

func (m *mockAccountRepo) Get(_ context.Context, userID domain.UserID) (*domain.Account, error) {
	acc, ok := m.accounts[string(userID)]
	if !ok {
		return nil, domain.ErrAccountNotFound
	}
	// Return the actual pointer so mutations affect the stored account
	return acc, nil
}

func (m *mockAccountRepo) UpdateBalance(_ context.Context, account *domain.Account) error {
	acc, ok := m.accounts[string(account.UserID)]
	if !ok {
		return domain.ErrAccountNotFound
	}
	if acc.Version != account.Version {
		return domain.ErrConcurrentModification
	}
	acc.Balance = account.Balance
	acc.Version++
	account.Version = acc.Version
	return nil
}

// mockTxRepo implements TransactionRepository for testing.
type mockTxRepo struct {
	transactions map[domain.TransactionID]*domain.Transaction
}

func newMockTxRepo() *mockTxRepo {
	return &mockTxRepo{
		transactions: make(map[domain.TransactionID]*domain.Transaction),
	}
}

func (m *mockTxRepo) Create(_ context.Context, tx *domain.Transaction) error {
	m.transactions[tx.ID] = tx
	return nil
}

func (m *mockTxRepo) Get(_ context.Context, id domain.TransactionID) (*domain.Transaction, error) {
	tx, ok := m.transactions[id]
	if !ok {
		return nil, domain.ErrTransactionNotFound
	}
	return tx, nil
}

func (m *mockTxRepo) GetRefundForOriginal(_ context.Context, originalID domain.TransactionID) (*domain.Transaction, error) {
	for _, tx := range m.transactions {
		if tx.OriginalTransactionID != nil && *tx.OriginalTransactionID == originalID {
			return tx, nil
		}
	}
	return nil, domain.ErrTransactionNotFound
}

func (m *mockTxRepo) UpdateStatus(_ context.Context, id domain.TransactionID, status domain.TransactionStatus, failureReason string) error {
	tx, ok := m.transactions[id]
	if !ok {
		return domain.ErrTransactionNotFound
	}
	tx.Status = status
	tx.FailureReason = failureReason
	return nil
}

// mockEventPublisher implements EventPublisher for testing.
type mockEventPublisher struct {
	events []any
}

func newMockEventPublisher() *mockEventPublisher {
	return &mockEventPublisher{}
}

func (m *mockEventPublisher) PublishPaymentSucceeded(_ context.Context, event PaymentSucceededEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventPublisher) PublishPaymentFailed(_ context.Context, event PaymentFailedEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventPublisher) PublishRefundSucceeded(_ context.Context, event RefundSucceededEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventPublisher) PublishRefundFailed(_ context.Context, event RefundFailedEvent) error {
	m.events = append(m.events, event)
	return nil
}

func TestCharge_Success(t *testing.T) {
	accounts := newMockAccountRepo()
	txs := newMockTxRepo()
	events := newMockEventPublisher()
	svc := NewPaymentService(accounts, txs, events)

	result, err := svc.Charge(context.Background(), "order-1", "user-1", 5000)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusSucceeded, result.Status)
	assert.NotEmpty(t, result.TransactionID)

	// Check balance was deducted
	acc, _ := accounts.Get(context.Background(), "user-1")
	assert.Equal(t, int64(95000), acc.Balance)

	// Check event was published
	assert.Len(t, events.events, 1)
	_, ok := events.events[0].(PaymentSucceededEvent)
	assert.True(t, ok)
}

func TestCharge_InsufficientFunds(t *testing.T) {
	accounts := newMockAccountRepo()
	txs := newMockTxRepo()
	events := newMockEventPublisher()
	svc := NewPaymentService(accounts, txs, events)

	result, err := svc.Charge(context.Background(), "order-2", "user-3", 5000)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusFailed, result.Status)
	assert.Equal(t, "insufficient funds", result.Message)

	// Check event was published
	assert.Len(t, events.events, 1)
	_, ok := events.events[0].(PaymentFailedEvent)
	assert.True(t, ok)
}

func TestCharge_InvalidAmount(t *testing.T) {
	accounts := newMockAccountRepo()
	txs := newMockTxRepo()
	events := newMockEventPublisher()
	svc := NewPaymentService(accounts, txs, events)

	result, err := svc.Charge(context.Background(), "order-3", "user-1", 0)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusFailed, result.Status)
}

func TestCharge_AccountNotFound(t *testing.T) {
	accounts := newMockAccountRepo()
	txs := newMockTxRepo()
	events := newMockEventPublisher()
	svc := NewPaymentService(accounts, txs, events)

	result, err := svc.Charge(context.Background(), "order-4", "nonexistent", 5000)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusFailed, result.Status)
	assert.Equal(t, "account not found", result.Message)
}

func TestRefund_Success(t *testing.T) {
	accounts := newMockAccountRepo()
	txs := newMockTxRepo()
	events := newMockEventPublisher()
	svc := NewPaymentService(accounts, txs, events)

	// First charge
	chargeResult, err := svc.Charge(context.Background(), "order-5", "user-1", 10000)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusSucceeded, chargeResult.Status)

	// Then refund
	refundResult, err := svc.Refund(context.Background(), "order-5", "user-1", 10000, chargeResult.TransactionID)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusSucceeded, refundResult.Status)

	// Check balance was restored
	acc, _ := accounts.Get(context.Background(), "user-1")
	assert.Equal(t, int64(100000), acc.Balance)

	// Check refund event was published
	assert.Len(t, events.events, 2) // charge + refund
	_, ok := events.events[1].(RefundSucceededEvent)
	assert.True(t, ok)
}

func TestRefund_Idempotency(t *testing.T) {
	accounts := newMockAccountRepo()
	txs := newMockTxRepo()
	events := newMockEventPublisher()
	svc := NewPaymentService(accounts, txs, events)

	// First charge
	chargeResult, err := svc.Charge(context.Background(), "order-6", "user-1", 10000)
	require.NoError(t, err)

	// First refund - success
	refundResult, err := svc.Refund(context.Background(), "order-6", "user-1", 10000, chargeResult.TransactionID)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusSucceeded, refundResult.Status)

	// Second refund with same original_transaction_id - should fail (already refunded)
	refundResult2, err := svc.Refund(context.Background(), "order-6", "user-1", 10000, chargeResult.TransactionID)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusFailed, refundResult2.Status)
	assert.Equal(t, "transaction has already been refunded", refundResult2.Message)
}

func TestRefund_InvalidOriginalTransaction(t *testing.T) {
	accounts := newMockAccountRepo()
	txs := newMockTxRepo()
	events := newMockEventPublisher()
	svc := NewPaymentService(accounts, txs, events)

	result, err := svc.Refund(context.Background(), "order-7", "user-1", 5000, "nonexistent-tx")
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusFailed, result.Status)
	assert.Equal(t, "original transaction not found", result.Message)
}

// TestConcurrentCharge_OptimisticLock is tested via integration tests with real PostgreSQL.

func TestCharge_ExactBalance(t *testing.T) {
	accounts := newMockAccountRepo()
	txs := newMockTxRepo()
	events := newMockEventPublisher()
	svc := NewPaymentService(accounts, txs, events)

	// user-2 has 50000, charge exactly that
	result, err := svc.Charge(context.Background(), "order-9", "user-2", 50000)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusSucceeded, result.Status)

	// Balance should be 0
	acc, _ := accounts.Get(context.Background(), "user-2")
	assert.Equal(t, int64(0), acc.Balance)
}

func TestRefund_InvalidAmount(t *testing.T) {
	accounts := newMockAccountRepo()
	txs := newMockTxRepo()
	events := newMockEventPublisher()
	svc := NewPaymentService(accounts, txs, events)

	result, err := svc.Refund(context.Background(), "order-10", "user-1", 0, "some-tx")
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusFailed, result.Status)
}

// Ensure mock implements interfaces
var (
	_ AccountRepository     = (*mockAccountRepo)(nil)
	_ TransactionRepository = (*mockTxRepo)(nil)
	_ EventPublisher        = (*mockEventPublisher)(nil)
)

// Suppress unused import
var _ = errors.New
