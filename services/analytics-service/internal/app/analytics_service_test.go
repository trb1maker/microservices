package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/services/analytics-service/internal/app"
)

type memoryReceipts struct {
	items map[string]app.Receipt
}

func (m *memoryReceipts) Exists(_ context.Context, orderID string) (bool, error) {
	_, ok := m.items[orderID]
	return ok, nil
}

func (m *memoryReceipts) Save(_ context.Context, receipt app.Receipt) error {
	if m.items == nil {
		m.items = map[string]app.Receipt{}
	}
	m.items[receipt.OrderID] = receipt
	return nil
}

type memorySummary struct {
	orders map[string]struct{}
	total  int64
}

func (m *memorySummary) RecordOrder(_ context.Context, orderID string, amount int64, _ time.Time) (bool, error) {
	if m.orders == nil {
		m.orders = map[string]struct{}{}
	}
	if _, ok := m.orders[orderID]; ok {
		return true, nil
	}
	m.orders[orderID] = struct{}{}
	m.total += amount
	return false, nil
}

func TestAnalyticsService_ProcessOrderFinalized(t *testing.T) {
	t.Parallel()
	receipts := &memoryReceipts{}
	summary := &memorySummary{}
	svc := app.NewAnalyticsService(receipts, summary)

	event := app.OrderFinalized{
		OrderID:     "order-1",
		UserID:      "user-1",
		TotalAmount: 1000,
		Status:      "CONFIRMED",
		FinalizedAt: time.Now().UTC().Format(time.RFC3339),
	}
	require.NoError(t, svc.ProcessOrderFinalized(context.Background(), event))
	require.NoError(t, svc.ProcessOrderFinalized(context.Background(), event))

	assert.Len(t, receipts.items, 1)
	assert.Len(t, summary.orders, 1)
	assert.Equal(t, int64(1000), summary.total)
}

func TestAnalyticsService_InvalidPayload(t *testing.T) {
	t.Parallel()
	svc := app.NewAnalyticsService(&memoryReceipts{}, &memorySummary{})
	err := svc.ProcessOrderFinalized(context.Background(), app.OrderFinalized{})
	assert.Error(t, err)
}
