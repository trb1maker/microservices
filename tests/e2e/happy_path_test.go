// go:build integration

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
