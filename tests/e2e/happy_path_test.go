//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_HappyPath(t *testing.T) {
	env := newEnv(t)

	orderID := env.checkoutOrder(t)

	payResp := env.doJSON(t, http.MethodPost, "/orders/"+orderID+"/pay", env.token, "")
	require.Equal(t, http.StatusOK, payResp.StatusCode)
	var paid struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(payResp.Body).Decode(&paid))
	_ = payResp.Body.Close()
	assert.Equal(t, "PAID", paid.Status)

	env.waitForOrderStatus(t, orderID, "CONFIRMED")

	require.Eventually(t, func() bool {
		return env.notification.Sink().HasEvent("ORDER_FINALIZED")
	}, 10*time.Second, pollInterval)

	require.Eventually(t, func() bool {
		exists, err := env.analytics.Storage().Exists(context.Background(), orderID)
		return err == nil && exists
	}, 10*time.Second, pollInterval)

	object, err := env.minioClient.GetObject(context.Background(), minioBucket, "receipts/"+orderID+".json", minio.GetObjectOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = object.Close() })
	var receipt struct {
		OrderID     string `json:"order_id"`
		TotalAmount int64  `json:"total_amount"`
	}
	require.NoError(t, json.NewDecoder(object).Decode(&receipt))
	assert.Equal(t, orderID, receipt.OrderID)
	assert.Equal(t, testUnitPrice, receipt.TotalAmount)

	require.Eventually(t, func() bool {
		return env.countReceiptDocuments(t, orderID) == 1
	}, 10*time.Second, pollInterval)

	receiptResp := env.doAnalyticsJSON(t, http.MethodGet, "/receipts/"+orderID, env.token, "")
	require.Equal(t, http.StatusOK, receiptResp.StatusCode)
	var receiptURL struct {
		URL       string `json:"url"`
		ExpiresIn int64  `json:"expires_in"`
	}
	require.NoError(t, json.NewDecoder(receiptResp.Body).Decode(&receiptURL))
	_ = receiptResp.Body.Close()
	assert.Contains(t, receiptURL.URL, orderID)

	searchResp := env.doAnalyticsJSON(t, http.MethodGet, "/receipts/search?q=Moscow", env.token, "")
	require.Equal(t, http.StatusOK, searchResp.StatusCode)
	var searchBody struct {
		Results []struct {
			OrderID string `json:"order_id"`
		} `json:"results"`
	}
	require.NoError(t, json.NewDecoder(searchResp.Body).Decode(&searchBody))
	_ = searchResp.Body.Close()
	require.Len(t, searchBody.Results, 1)
	assert.Equal(t, orderID, searchBody.Results[0].OrderID)

	assert.Equal(t, 1, env.countProcessedOrders(t, orderID))
	assert.Equal(t, 1, env.countPaymentTransactions(t, orderID))
	assert.Equal(t, 0, env.stockReserved(t))

	beforeSummary := env.dailySummaryOrders(t)
	env.publishDuplicateFinalized(t, orderID)
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, beforeSummary, env.dailySummaryOrders(t))
	assert.Equal(t, 1, env.countProcessedOrders(t, orderID))
}

func (e *env) dailySummaryOrders(t *testing.T) int {
	t.Helper()
	var total int
	err := e.analyticsPool.QueryRow(context.Background(), `
		SELECT COALESCE(total_orders, 0)
		FROM daily_summary
		WHERE date = CURRENT_DATE
	`).Scan(&total)
	if err != nil {
		return 0
	}
	return total
}
