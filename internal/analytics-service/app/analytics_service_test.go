package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
)

type memoryReceipts struct {
	items map[string]app.Receipt
	urls  map[string]string
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

func (m *memoryReceipts) PresignGet(_ context.Context, orderID string, _ time.Duration) (string, error) {
	if m.urls == nil {
		return "", app.ErrReceiptNotFound
	}
	url, ok := m.urls[orderID]
	if !ok {
		return "", app.ErrReceiptNotFound
	}
	return url, nil
}

type memorySummary struct {
	orders map[string]struct{}
	total  int64
}

func (m *memorySummary) IsOrderProcessed(_ context.Context, orderID string) (bool, error) {
	if m.orders == nil {
		return false, nil
	}
	_, ok := m.orders[orderID]
	return ok, nil
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

type memoryDocuments struct {
	docs map[string]app.ReceiptDocument
}

func (m *memoryDocuments) Upsert(_ context.Context, doc app.ReceiptDocument) error {
	if m.docs == nil {
		m.docs = map[string]app.ReceiptDocument{}
	}
	m.docs[doc.OrderID] = doc
	return nil
}

func (m *memoryDocuments) GetByOrderID(_ context.Context, orderID string) (*app.ReceiptDocument, error) {
	doc, ok := m.docs[orderID]
	if !ok {
		return nil, nil
	}
	return &doc, nil
}

func (m *memoryDocuments) Search(_ context.Context, userID, query string, limit int) ([]app.SearchResult, error) {
	results := make([]app.SearchResult, 0)
	for _, doc := range m.docs {
		if doc.UserID != userID {
			continue
		}
		if query != "" && !containsFold(doc.SearchText, query) {
			continue
		}
		results = append(results, app.SearchResult{
			OrderID:         doc.OrderID,
			UserID:          doc.UserID,
			TotalAmount:     doc.TotalAmount,
			Status:          doc.Status,
			FinalizedAt:     doc.FinalizedAt,
			DeliveryAddress: doc.DeliveryAddress,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func containsFold(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) &&
		(needle == haystack || len(haystack) > 0 && searchFold(haystack, needle))
}

func searchFold(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if equalFold(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

const (
	testOrderID = "order-1"
	testUserID  = "user-1"
)

func TestAnalyticsService_ProcessOrderFinalized(t *testing.T) {
	t.Parallel()
	receipts := &memoryReceipts{}
	summary := &memorySummary{}
	documents := &memoryDocuments{}
	svc := app.NewAnalyticsService(receipts, summary, documents, time.Minute)

	event := app.OrderFinalized{
		OrderID:         testOrderID,
		UserID:          testUserID,
		TotalAmount:     1000,
		Status:          "CONFIRMED",
		FinalizedAt:     time.Now().UTC().Format(time.RFC3339),
		DeliveryAddress: "Moscow",
	}
	require.NoError(t, svc.ProcessOrderFinalized(context.Background(), event))
	require.NoError(t, svc.ProcessOrderFinalized(context.Background(), event))

	assert.Len(t, receipts.items, 1)
	assert.Len(t, summary.orders, 1)
	assert.Equal(t, int64(1000), summary.total)
	assert.Contains(t, documents.docs[testOrderID].SearchText, "Moscow")
}

func TestAnalyticsService_InvalidPayload(t *testing.T) {
	t.Parallel()
	svc := app.NewAnalyticsService(&memoryReceipts{}, &memorySummary{}, &memoryDocuments{}, time.Minute)
	err := svc.ProcessOrderFinalized(context.Background(), app.OrderFinalized{})
	assert.Error(t, err)
}

func TestAnalyticsService_GetReceiptURL(t *testing.T) {
	t.Parallel()
	receipts := &memoryReceipts{
		items: map[string]app.Receipt{
			testOrderID: {OrderID: testOrderID},
		},
		urls: map[string]string{testOrderID: "https://minio/receipts/order-1.json"},
	}
	documents := &memoryDocuments{
		docs: map[string]app.ReceiptDocument{
			testOrderID: {OrderID: testOrderID, UserID: testUserID},
		},
	}
	svc := app.NewAnalyticsService(receipts, &memorySummary{}, documents, app.DefaultReceiptURLTTL)

	url, ttl, err := svc.GetReceiptURL(context.Background(), testUserID, testOrderID)
	require.NoError(t, err)
	assert.Equal(t, "https://minio/receipts/order-1.json", url)
	assert.Equal(t, app.DefaultReceiptURLTTL, ttl)

	_, _, err = svc.GetReceiptURL(context.Background(), "user-2", testOrderID)
	assert.ErrorIs(t, err, app.ErrReceiptNotFound)
}

func TestAnalyticsService_SearchReceipts(t *testing.T) {
	t.Parallel()
	documents := &memoryDocuments{
		docs: map[string]app.ReceiptDocument{
			testOrderID: {
				OrderID:         testOrderID,
				UserID:          testUserID,
				DeliveryAddress: "Moscow Red Square",
				SearchText:      testOrderID + " " + testUserID + " Moscow Red Square CONFIRMED",
			},
		},
	}
	svc := app.NewAnalyticsService(&memoryReceipts{}, &memorySummary{}, documents, time.Minute)

	results, err := svc.SearchReceipts(context.Background(), testUserID, "Moscow", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	_, err = svc.SearchReceipts(context.Background(), "user-1", " ", 10)
	assert.ErrorIs(t, err, app.ErrSearchQueryRequired)
}
